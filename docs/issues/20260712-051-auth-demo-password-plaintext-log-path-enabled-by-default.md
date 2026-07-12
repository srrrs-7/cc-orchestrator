---
id: ISSUE-051
title: app/auth にデモパスワードを平文でログ出力するコードパスがあり、ローカル compose で既定有効になっている
status: open  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-12
updated: 2026-07-12
specs: [SPEC-005]  # 関連Spec ID (例: [SPEC-002])
---

# ISSUE-051: app/auth にデモパスワードを平文ログ出力するコードパスがあり compose で既定有効

**深刻度: Minor(review) / severity: low**(値は既知デモ用途で実害限定的だが「秘密をログに出すコードパス」自体が存在する)

## 1. ユーザー価値への影響(なぜ対応するか)

> **app/auth の運用者・開発者** が **この平文ログ出力コードパスを本番相当環境で誤って有効化した場合に**、**秘密(パスワード)がログに漏えいする**。

- **影響を受けるユーザー**: 運用者・開発者(現状のデモ値自体の実害は限定的)
- **損なわれる価値**: 秘密情報の非ログ化(秘密をログに出さない原則)
- **影響範囲・頻度**: ローカル compose では既定で平文出力される。本番相当でフラグを誤設定すると漏えい直結
- **回避策**: あり(フラグを無効化すれば出力されない)。ただしコードパス自体が誤用リスク

## 2. 現象(何が起きているか)

### 期待する動作

秘密情報はログに平文で出さない(常にマスク値 `[redacted]` を出す、またはローカル専用に厳格ゲート)。

### 実際の動作

`app/auth/cmd/authz/main.go:147-149` に `slog.Info(... "password", demoPassword)` の平文ログ出力がある。`compose.yml:119-120` で `DEMO_PASSWORD: demo` + `DEMO_LOG_PASSWORD: "1"` が設定され、ローカル compose では既定でこのコードパスが有効になっている。

値は既知のデモ用途で実害は限定的だが、「秘密をログに出すコードパス」自体が存在し、誤用時に漏えいへ直結する。

### 再現手順

1. `app/auth/cmd/authz/main.go:147-149` の `slog.Info(... "password", demoPassword)` を確認する。
2. `compose.yml:119-120` の `DEMO_PASSWORD: demo` / `DEMO_LOG_PASSWORD: "1"` を確認する。
3. `make up` で auth を起動すると、ログにデモパスワードが平文で出力される。

### 環境・条件

- 対象: app/auth(cmd/authz)。ローカル compose で既定有効。

## 3. 原因(なぜ起きているか)

### 調査ログ

- 事実: `app/auth/cmd/authz/main.go:147-149` で demoPassword を slog に平文で渡す。
- 事実: `compose.yml:119-120` で `DEMO_LOG_PASSWORD: "1"` により既定有効。
- 仮説: ローカル開発の利便(デモ資格情報の周知)のために入れた出力が、秘密ログ化のコードパスとして残っている。

### 根本原因

秘密(パスワード)をログに平文で出力するコードパスが存在し、compose で既定有効になっている。

## 4. 対応(どう解決するか)

### 対応方針

impl-auth が、ログは常にマスク値(`[redacted]`)を出すようにする、または機能をローカル専用フラグでさらに厳格にゲートする。

### 実施内容

- [ ] `app/auth/cmd/authz/main.go:147-149` の平文出力をマスク値化、またはローカル専用に厳格ゲート(impl-auth)
- [ ] 必要なら compose.yml の `DEMO_LOG_PASSWORD` の扱いを見直し(impl-ci: ルート compose.yml 所有)
- [ ] 平文が出ないことを確認(tester)

### 再発防止

- 秘密情報をログに渡さない lint / レビュー観点を追加する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。セキュリティレビューで検出。`app/auth/cmd/authz/main.go:147-149` の平文ログ出力と `compose.yml:119-120` の既定有効フラグを確認した。
- 関連: ISSUE-005(デモユーザーのパスワード平文保持・比較、resolved)、SPEC-005。
