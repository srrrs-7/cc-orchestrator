---
id: ISSUE-043
title: app/iac の ISSUER が http スキームのため、IdP セッション Cookie に Secure 属性が付かず SSL ストリッピングでハイジャック可能
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: high  # critical | high | medium | low
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-043: app/iac の ISSUER が http スキームのため、IdP セッション Cookie に Secure 属性が付かない

**深刻度: Major(review) / severity: high**(IdP セッションの平文キャプチャ→セッションハイジャック)

## 1. ユーザー価値への影響(なぜ対応するか)

> **IdP(app/auth)にログイン済みのユーザー** の **セッションの機密性** が **Secure 属性欠如により中間者へ露出し、セッションハイジャックされ得る**。

- **影響を受けるユーザー**: 本番相当環境で IdP セッションを保持する全ユーザー
- **損なわれる価値**: セッションの機密性・アカウント制御。`idp_session` Cookie を平文キャプチャされると成りすまし可能
- **影響範囲・頻度**: SSL ストリッピング / 中間者が可能なネットワーク条件で発生(常時露出はしないが、Secure 欠如により防御の 1 層が無効)
- **回避策**: なし(ISSUER を https 化するまで Cookie は Secure:false のまま)

## 2. 現象(何が起きているか)

### 期待する動作

CloudFront が HTTPS 強制(`viewer_protocol_policy = redirect-to-https`)である以上、IdP セッション Cookie(`idp_session` / `idp_pending`)は `Secure` 属性付きで発行される。

### 実際の動作

`app/iac/envs/dev/main.tf:248` で `ISSUER = "http://${module.cdn.cloudfront_domain_name}/auth"` と http 固定で設定されている(確認済み)。

app/auth の `SecureCookiesFromIssuer`(`app/auth/route/router.go:89-92`)は issuer の scheme が `https://` で始まるかで Secure 属性を決める。ISSUER が `http://` のため、実際には HTTPS でしかアクセスされないにもかかわらず、セッション Cookie が `Secure: false` で発行される(`app/auth/route/session_cookie.go` の `idp_session` / `idp_pending`)。

結果、SSL ストリッピングや中間者攻撃で `idp_session` を平文キャプチャされ、セッションハイジャックされ得る。

### 再現手順

1. `app/iac/envs/dev/main.tf:248` の `ISSUER` が `http://...` であることを確認する。
2. デプロイ後、`https://<cloudfront_domain>/auth/...` でログインして `Set-Cookie: idp_session=...` を確認すると `Secure` 属性が付かない。

### 環境・条件

- 対象: CloudFront 経由でアクセスされる本番相当環境。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/iac/envs/dev/main.tf:248` の ISSUER が `http://` スキーム。
- 事実: `app/auth/route/router.go:89-92` の `SecureCookiesFromIssuer` は `strings.HasPrefix(strings.ToLower(issuer), "https://")` で判定する。http のため false。
- 事実: CloudFront は `redirect-to-https` で viewer に HTTPS を強制する(実アクセスは常に HTTPS)。
- 仮説: ISSUER の scheme(トークンの iss クレーム用)と、実際の viewer プロトコルが乖離しており、Cookie の Secure 判定を ISSUER に一本化した設計が本番の HTTPS 実態とズレている。

### 根本原因

ISSUER が http 固定であり、Cookie の Secure 判定がその scheme に依存しているため、HTTPS 実態と一致しない。

## 4. 対応(どう解決するか)

### 対応方針

impl-iac が ISSUER を https スキームに変更する。あわせて app/auth 側で X-Forwarded-Proto ベースの Secure 判定も検討する(ALB/CloudFront 背後でのプロトコル判定の堅牢化)。

### 実施内容

- [ ] `app/iac/envs/dev/main.tf:248` の `ISSUER` を `https://${module.cdn.cloudfront_domain_name}/auth` に変更(impl-iac)
- [ ] ISSUER 変更が OIDC discovery の `issuer` / トークンの `iss` クレームと整合することを確認(RP/リソースサーバー側の期待値と一致)
- [ ] app/auth の Secure 判定を X-Forwarded-Proto ベースにする案の要否を検討(impl-auth。要れば別途)
- [ ] apply は行わず plan 結果を報告する

### 再発防止

- ISSUER の scheme と viewer_protocol_policy の整合を review-iac/spec で点検する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。セキュリティレビューで検出。`app/iac/envs/dev/main.tf:248` の http ISSUER と `app/auth/route/router.go:89-92` の `SecureCookiesFromIssuer`(https prefix 判定)を実地確認した。CloudFront は redirect-to-https で実アクセスは HTTPS のため、Cookie が不要に Secure:false になる。
- 関連: ISSUE-014(iac デプロイ経路、resolved)、SPEC-015。
