---
id: ISSUE-042
title: app/auth の login/consent 画面にフレーム制御ヘッダが無く、本番アクセス経路(CloudFront /auth/*)でクリックジャッキングが成立する
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: critical  # critical | high | medium | low
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-014, SPEC-015]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-042: app/auth の login/consent 画面にフレーム制御ヘッダが無く、本番アクセス経路でクリックジャッキングが成立する

**深刻度: Blocker(review) / severity: critical**(クリックジャッキング経由のアカウント乗っ取りに直結)

## 1. ユーザー価値への影響(なぜ対応するか)

> **既に app/auth(IdP)にログイン済みのユーザー** の **アカウント(および連携先リソースへのアクセス)** が **透明 iframe を用いたクリックジャッキングで奪取される**。

- **影響を受けるユーザー**: 本番相当環境(CloudFront 経由)で IdP セッションを保持している全ユーザー
- **損なわれる価値**: アカウントの制御。攻撃者 client への認可コード発行に気付かず同意させられ、access/refresh token を奪取される(アカウント乗っ取り)
- **影響範囲・頻度**: 本番の実アクセス経路(CloudFront `/auth/*` → ALB → app/auth)で常時成立。攻撃には victim の誘導・クリックが必要
- **回避策**: なし(ヘッダを付与しない限りフレーム埋め込みを防げない)

## 2. 現象(何が起きているか)

### 期待する動作

app/auth の全レスポンス(特に login / consent 画面)が `X-Frame-Options: DENY` または CSP `frame-ancestors 'none'` を持ち、他オリジンの iframe への埋め込みを拒否する。本番経路でも同ポリシーが適用される。

### 実際の動作

app/auth はフレーム制御ヘッダ(`X-Frame-Options` / CSP `frame-ancestors` / `X-Content-Type-Options`)をどのレスポンスにも設定しない(確認済み: `app/auth/` 配下に該当ヘッダの grep ヒットなし)。

- ローカル compose では web nginx がヘッダを緩和的に補うが、本番の実アクセス経路 CloudFront `/auth/*` behavior → ALB → app/auth はこの nginx を通らない。
- 本番の CloudFront では `/auth/*` の `ordered_cache_behavior` に web 用の `response_headers_policy`(`aws_cloudfront_response_headers_policy.web_security`)が適用されていない(確認済み: `app/iac/modules/cdn/main.tf:248` に「/auth/* ordered behaviors intentionally omit this policy」のコメントがあり、policy は default behavior のみ)。

結果、攻撃者は透明 iframe に `/auth/consent`(または login)を埋め込み、Allow ボタンにクリックを誘導できる。既存 IdP セッションを保持する victim が気付かずに攻撃者 client への認可コード発行に同意 → 攻撃者が access/refresh token を取得しアカウント乗っ取りに至る。

### 再現手順

1. app/auth に対して `curl -I https://<cloudfront_domain>/auth/consent`(または login)を実行し、`X-Frame-Options` / `Content-Security-Policy: frame-ancestors` / `X-Content-Type-Options` が返らないことを確認する。
2. 攻撃者ページで `<iframe src="https://<cloudfront_domain>/auth/consent?...">` を透明化してオーバーレイし、Allow ボタン位置に囮の UI を重ねる。
3. IdP ログイン済みの victim がクリックすると consent が送信され、認可コードが攻撃者 client に発行される。

### 環境・条件

- 対象: 本番の CloudFront `/auth/*` 経路(app/auth 直アクセス)。ローカル compose は web nginx が緩和するため顕在化しにくい。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/auth/` 配下に `X-Frame-Options` / `frame-ancestors` / `X-Content-Type-Options` / `Content-Security-Policy` を設定するコードが存在しない(grep で 0 件)。
- 事実: `app/iac/modules/cdn/main.tf:32-62` の `web_security` response_headers_policy は default(S3)behavior にのみ適用され、`/auth/*` の ordered_cache_behavior(main.tf:287 付近)には適用されない(main.tf:248 のコメントが意図的除外を明記)。
- 仮説: フレーム制御を web nginx / CloudFront に暗黙依存していたが、app/auth の本番直アクセス経路がその防御層の外にあるため多層防御が破れている。

### 根本原因

app/auth 自身がフレーム制御ヘッダを一切付与せず、かつ本番の `/auth/*` 経路に相当の response_headers_policy が適用されていない(防御が単一層に依存し、その層を通らない経路が存在する)。

## 4. 対応(どう解決するか)

### 対応方針(修正は 2 箇所にまたがる)

多層防御として app/auth 自身にヘッダ付与を追加し、加えて CloudFront `/auth/*` にも同等ポリシーを適用する。

### 実施内容

- [ ] **app/auth**(impl-auth): 全レスポンスに `X-Frame-Options: DENY` / `Content-Security-Policy: frame-ancestors 'none'` / `X-Content-Type-Options: nosniff` を付与するミドルウェアを `app/auth/route/` に新設し、router に配線する(nginx/CloudFront に非依存の多層防御)
- [ ] **app/iac**(impl-iac): `app/iac/modules/cdn/main.tf` の `/auth/*` `ordered_cache_behavior`(main.tf:287 付近)に同等の `response_headers_policy` を適用する。login/consent のフォーム送信が壊れないよう CSP は `form-action 'self'` 等、必要な範囲へ調整する
- [ ] 修正後、本番経路(CloudFront `/auth/*`)でヘッダが返ることを確認する

### 再発防止

- app/auth を「防御ヘッダを自前で持つ」前提にし、nginx/CloudFront はあくまで多層目にする。
- CloudFront の各 behavior にセキュリティヘッダポリシーが適用されているかを review-spec/iac で点検する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。リポジトリ全体のセキュリティレビューで検出。`app/auth/` にフレーム制御ヘッダのコードが無いこと、`app/iac/modules/cdn/main.tf:248` が `/auth/*` への web_security policy 適用を意図的に除外していることを実地確認した。
- 関連: SPEC-014(app/web CSP 導入)、SPEC-015(OIDC 認証連携)。
