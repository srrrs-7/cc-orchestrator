---
id: ISSUE-051
title: app/auth にデモパスワードを平文でログ出力するコードパスがあり、ローカル compose で既定有効になっている
status: resolved  # open | investigating | fixing | resolved | closed | wontfix
severity: low  # critical | high | medium | low
created: 2026-07-12
updated: 2026-07-25
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
- 事実(2026-07-25 追記): 現在の該当箇所は `app/auth/cmd/authz/main.go:158-160`(行番号がシフト)。条件は `if e.DemoPassword != "" || os.Getenv("DEMO_LOG_PASSWORD") == "1"` で、`e.DemoPassword` が非空なだけで `slog.Info("authz: demo user password", "username", demoUsername, "password", demoPassword)` が平文出力される。`|| DEMO_LOG_PASSWORD == "1"` は実質 no-op な冗長条件で、`DEMO_LOG_PASSWORD` を設定しない運用でも `DEMO_PASSWORD` に実資格情報を入れた時点で漏えい経路になる(compose の `DEMO_LOG_PASSWORD: "1"` 有無に依らず出力される)。

### 根本原因

秘密(パスワード)をログに平文で出力するコードパスが存在し、compose で既定有効になっている。

## 4. 対応(どう解決するか)

### 対応方針

**実装計画: [`docs/plans/ISSUE-056-plan.md`](../plans/ISSUE-056-plan.md)**(ISSUE-056 の runtime コンテナ最小権限ハードニングに同梱。同一 review-security 由来の一括対応)。impl-auth が `DEMO_LOG_PASSWORD` を `cmd/authz/env.go` の `Env`(bool `LogDemoPassword`)へ移し、`main.go` のログ条件を `if e.LogDemoPassword {`(= `DEMO_LOG_PASSWORD == "1"` 単独)に一本化する。`DEMO_PASSWORD` 単独では出力しない。compose の `DEMO_LOG_PASSWORD: "1"` は「既知・非機密デモ資格情報の明示的ローカル opt-in」として残す(除去は不要。impl-ci 判断)。検証は `env_test.go`(`DEMO_LOG_PASSWORD` 写像 table test)+ `make up-d` のログ観察(フラグ無しで平文が出ないこと)。

impl-auth が、ログは常にマスク値(`[redacted]`)を出すようにする、または機能をローカル専用フラグでさらに厳格にゲートする。

### 実施内容

- [x] ログ出力を明示的 opt-in に厳格ゲート(impl-auth。`cmd/authz/main.go:163` を `if e.LogDemoPassword {` に変更)
- [x] ログ出力条件を明示的な opt-in(`DEMO_LOG_PASSWORD == "1"` のみ)に一本化し、`e.DemoPassword != ""` 単独では出力しない。既定では平文パスワードを info ログに出さない(impl-auth。`cmd/authz/env.go:113` で `LogDemoPassword: os.Getenv("DEMO_LOG_PASSWORD") == "1"`)
- [x] compose.yml の `DEMO_LOG_PASSWORD` の扱いを見直し(impl-ci: 「既知・非機密デモ資格情報の明示的ローカル opt-in」として残し、注記を追加)
- [x] 平文が出ないことを確認(tester。`env_test.go:417` の `TestNewEnv_LogDemoPassword` 追加 + `make up-d` の実挙動で `DEMO_PASSWORD` 単独では非出力を確認)

### 再発防止

- 秘密情報をログに渡さない lint / レビュー観点を追加する。

## 5. 経緯(時系列・追記のみ)

### 2026-07-12

- 起票。セキュリティレビューで検出。`app/auth/cmd/authz/main.go:147-149` の平文ログ出力と `compose.yml:119-120` の既定有効フラグを確認した。
- 関連: ISSUE-005(デモユーザーのパスワード平文保持・比較、resolved)、SPEC-005。

### 2026-07-25

- 発見経緯: review-security によるコンテナ最小権限レビュー(2026-07-25)で本コードパスが再度指摘された。新規起票せず本 Issue(同一コードパス・同一ファイル・open のまま)に統括して追記する。
- 追加で判明した事実: 現在の該当箇所は `app/auth/cmd/authz/main.go:158-160` にシフト。条件 `if e.DemoPassword != "" || os.Getenv("DEMO_LOG_PASSWORD") == "1"` により、`DEMO_LOG_PASSWORD` を設定しなくても `DEMO_PASSWORD` が非空なだけで平文出力される(`DEMO_LOG_PASSWORD` は実質 no-op な冗長条件)。起票時(2026-07-12)は compose の `DEMO_LOG_PASSWORD: "1"` を既定有効のトリガと捉えていたが、実際は `DEMO_PASSWORD` 単独でもトリガする点が精緻化された。
- 期待動作の明確化: ログ出力条件を明示的な opt-in(`DEMO_LOG_PASSWORD == "1"` のみ)に一本化し、既定では平文パスワードを info ログに出さない。実施内容にチェック項目を追記した(担当 impl-auth)。severity は low を維持(現状値はローカル固定の `"demo"` で実害限定的。ただし実資格情報を設定する運用変更が入ると漏えい経路になる)。
- 実装計画を [`docs/plans/ISSUE-056-plan.md`](../plans/ISSUE-056-plan.md) に策定(planner)。ISSUE-056(runtime コンテナ最小権限ハードニング)に同梱。方針: (1) `DEMO_LOG_PASSWORD` を `cmd/authz/main.go` のインライン `os.Getenv` から `cmd/authz/env.go` の `Env` bool フィールド `LogDemoPassword` へ移し(env 読み取り一本化規約に整合・単体テスト可能な seam 化)、`run()` の条件を `if e.LogDemoPassword {` に変更 = `DEMO_LOG_PASSWORD == "1"` 単独 opt-in、(2) compose の `DEMO_LOG_PASSWORD: "1"` は明示的ローカル opt-in として残す(コード側でゲート済みのため矛盾しない。除去したい場合は impl-ci が compose から外す)、(3) tester が `env_test.go` に写像 table test を追加(`DEMO_PASSWORD` 非空だけでは `LogDemoPassword=false` を回帰固定)。status は実装未完のため open 維持。

### 2026-07-25(実装・検証完了 → resolved)

- 実装(impl-auth。ISSUE-056 と同一 plan の 4 レーン並列で完了): `cmd/authz/env.go` の `Env` に bool `LogDemoPassword` を追加し、`newEnv()` で `LogDemoPassword: os.Getenv("DEMO_LOG_PASSWORD") == "1"`(113 行)に一本化。`cmd/authz/main.go` のインライン `os.Getenv` 条件を撤去し、平文ログ出力を `if e.LogDemoPassword {`(163 行)= `DEMO_LOG_PASSWORD == "1"` 単独 opt-in に変更。これにより起票時の症状(`e.DemoPassword != ""` 単独=`DEMO_PASSWORD` 設定だけで平文出力される冗長条件)を除去した。
- compose(impl-ci): `compose.yml` の `DEMO_LOG_PASSWORD: "1"`(226 行)は「既知・非機密デモ資格情報の明示的ローカル opt-in」として残し、その旨の注記(222-226 行)を追加。コード側でゲート済みのため矛盾しない。
- テスト検証(事実): tester が `cmd/authz/env_test.go:417` に `TestNewEnv_LogDemoPassword`(table test。`DEMO_PASSWORD` 非空だけでは `LogDemoPassword=false` を回帰固定するケースを含む)を追加し、auth の `make test` を含む `make check` が 15/15 pass・green(checker)。
- 実挙動検証(事実): `make up-d`(Phase 3 統合検証)で、`DEMO_PASSWORD` 単独設定ではパスワードがログに非出力・`DEMO_LOG_PASSWORD=1` を明示したときのみ平文出力されることを実挙動で確認。
- レビュー検証(事実): review-security 再レビューで本コードパスに関する Blocker / Major / Minor 指摘ゼロ。
- 判定: 「秘密(パスワード)をログに平文出力するコードパスが既定で有効」という症状を、明示 opt-in への一本化 + 回帰テスト固定 + 実挙動確認 + review-security 指摘ゼロで検証できたため status を `resolved` に遷移。severity は low を維持(現状値はローカル固定の `"demo"`)。
