---
id: ISSUE-046
title: SPEC-015 R13 の env 契約が 2 変数 all-or-nothing のままで、ISSUE-037 で追加された AUTH_AUDIENCE(3 変数化)が未反映(仕様の陳腐化)
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-015]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-046: SPEC-015 R13 の env 契約に AUTH_AUDIENCE が未反映(仕様の陳腐化)

**深刻度: Major(review) / severity: low**(実装は正常。仕様の環境変数契約表が古く、配線者を誤誘導するリスク)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/api / app/iac の配線担当・レビュアー** が **SPEC-015 R13 の環境変数契約表を信頼して認証 env を設定する際に**、**AUTH_AUDIENCE の欠落した古い契約に従い誤配線する**。

- **影響を受けるユーザー**: 開発者・配線担当・レビュアー(エンドユーザーへの直接影響はない)
- **損なわれる価値**: env 契約ドキュメントの正確性(一次情報としての価値)
- **影響範囲・頻度**: SPEC-015 R13 の環境変数契約表を参照して認証 env を設定するときに顕在化
- **回避策**: あり(実装 `env.go` を確認すれば 3 変数だと分かる)。ただし「仕様を正とする」原則に反する状態

## 2. 現象(何が起きているか)

### 期待する動作

SPEC-015 R13 の env 契約(環境変数契約表)が現実装と一致し、`AUTH_ISSUER` / `AUTH_JWKS_URL` / `AUTH_AUDIENCE` の 3 変数 all-or-nothing として記載される。

### 実際の動作

`docs/specs/20260712-015-web-oidc-authentication.md` の R13 / 環境変数契約表は **2 変数(AUTH_ISSUER / AUTH_JWKS_URL)の all-or-nothing** と記載している。

一方、実装は ISSUE-037 で `AUTH_AUDIENCE` を含む **3 変数の all-or-nothing 検証**に変更済み(`app/api/cmd/api/env.go:164-176`、テスト `app/api/cmd/api/env_test.go:462-496`)。環境変数契約表にも `AUTH_AUDIENCE` の記載が無い。

### 再現手順

1. `docs/specs/20260712-015-web-oidc-authentication.md` の R13 / 環境変数契約表を読む(2 変数記載)。
2. `app/api/cmd/api/env.go:164-176` の validate が `AUTH_ISSUER` / `AUTH_JWKS_URL` / `AUTH_AUDIENCE` の 3 変数 all-or-nothing であることを確認する。
3. 記述と実装が矛盾していることを確認する。

### 環境・条件

- ドキュメント上の矛盾(実行時の不具合ではない)。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: SPEC-015 R13 / 環境変数契約表は 2 変数 all-or-nothing と記載。
- 事実: `app/api/cmd/api/env.go:164-176` は AUTH_AUDIENCE を含む 3 変数の all-or-nothing 検証(ISSUE-037 で追加)。
- 事実: `app/api/cmd/api/env_test.go:462-496` が 3 変数検証をカバー。
- 仮説: ISSUE-037 で AUTH_AUDIENCE を追加した際、SPEC-015 の env 契約表への反映が漏れた。

### 根本原因

ISSUE-037 の実装で env 契約が 3 変数化したが、SPEC-015 R13 / 環境変数契約表が 2 変数のまま更新されず陳腐化した。

## 4. 対応(どう解決するか)

### 対応方針

**これは仕様側の陳腐化であり、修正は該当 Spec(SPEC-015、または ISSUE-037 が求めた後継 Spec)の更新**で行う。R13 / 環境変数契約表に `AUTH_AUDIENCE` を含む 3 変数契約を反映し、経緯セクションに追記する。

> 注記: 本 Issue の修正は、admin が `spec` skill で SPEC-015 を更新して閉じる予定。コード変更は不要(コードは正しい)。

### 実施内容

- [ ] admin が `spec` skill で SPEC-015 R13 / 環境変数契約表を 3 変数(AUTH_ISSUER / AUTH_JWKS_URL / AUTH_AUDIENCE)all-or-nothing へ更新
- [ ] SPEC-015 の経緯セクションに、ISSUE-037 による AUTH_AUDIENCE 追加を追記
- [ ] 更新後、本 Issue を resolved に更新

### 再発防止

- env 契約を変える実装(ISSUE-037 等)は、起点 Spec の環境変数契約表・経緯更新を必須とする。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。仕様準拠レビューで検出。SPEC-015 R13 の 2 変数記載と、`app/api/cmd/api/env.go:164-176`(3 変数 all-or-nothing、ISSUE-037 で追加)/ `env_test.go:462-496` の矛盾を確認した。契約表に AUTH_AUDIENCE の記載が無い。
- 関連: ISSUE-037(リソースサーバー audience 設計、resolved)、SPEC-015。修正は admin の spec skill による SPEC-015 更新で行う。
