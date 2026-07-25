---
paths:
  - "app/**"
---

# テスト規約

## 共通

- テストは実装と同じ Spec / Issue の作業内で必ず追加する。テストなしで完了にしない
- テストは Spec の機能要件(R1…)/ Issue の期待動作と対応付ける(どのテストがどの要件を検証するかを明確に)
- 観点は 正常系 / 異常系 / 境界値 の 3 つを最低限カバーする
- 落ちているテストを skip・削除して「通した」ことにしない。落ちる理由を報告する
- 実行順序に依存するテスト、実時間 sleep に依存するテストを書かない

## stack ごと

| stack | フレームワーク | 方針 |
|---|---|---|
| web | Vitest + React Testing Library | ユーザー視点(role / label)でクエリする。実装詳細(class 名・内部 state)に依存しない |
| api | 標準 `go test` | table-driven test を基本形にする。**DB は実 test DB(`api_test` / `auth_test`)に対して回す**(SPEC-013。モックしない)。DB 以外の外部 API は interface 越しに fake へ差し替える。実 DB が観測できない実装 seam(reader/writer 振り分け等)の検証は、実 repo をラップする最小の計装に留める |
| iac | `terraform validate` + `terraform plan` | plan 結果の差分が意図通りかを確認する。破壊的変更(replace)は必ず報告する |

## 永続化リポジトリのふるまい契約テスト(api / auth)

**ふるまい契約テストを実装ごとに二重化しない**。ドメインが定義する `Repository` を単一の正とし、`infra/repotest` パッケージに `Run<集約>RepositoryContract(t, newRepo)` を 1 つだけ書いて各実装のテストから呼ぶ:

- `infra/postgres/<集約>_repository_integration_test.go` — 実 test DB(`api_test` / `auth_test`)に対して回す。**`//go:build integration` タグは廃止済み**(SPEC-013。default `make test` / `make check` の一部として実 DB で実行される)。実行契約の正は `.claude/rules/db.md`
- `infra/repotest` は標準ライブラリと対象ドメインパッケージ以外に依存しない(SPEC-011 で infra/memory 削除済み。契約は Postgres 実装のみで実行)

## テストの実 DB 一本化(SPEC-013)

DB に触れるテストは、モック / 手書きダブルではなく **実 test DB**(`api_test` / `auth_test`。開発用 `api` / `auth` とは別データベースで、テスト実行で汚染しない)に対して実行する。`//go:build integration` による offline / DB の 2 層分離は廃止し、一本化した:

- **DB 依存テスト**(infra/postgres のリポジトリ・`OpenPair` 2 プール・`infra/repotest` 契約・route の正常系 / 全フロー・auth の authorize / token / refresh フロー・service の機能系)は、`testsupport.OpenTestDB(t)` で実 test DB に接続し、実 `postgres.New*Repository` を配線する。default `make test` / `make check` の一部。
- **隔離**: 各テスト冒頭で truncate + seed。DB 到達 run は `go test -p 1`(パッケージ間シリアライズ)で回す。`t.Parallel()` は新たに足さない(将来 `t.Parallel()` を導入するなら truncate + 共有 DB は破綻するため、並列単位ごとの別 DB へ移行する。SPEC-013 §6)。
- **fail-closed**: `testsupport.RequireDBHost(t)`(`OpenTestDB` も内部で使用)が、`DB_HOST` 未設定時に `REQUIRE_DB=1` なら `t.Fatal`(黙って skip させない)、`REQUIRE_DB` 未設定なら `t.Skip`。正規経路(CI / pre-commit / ルート経由 test)は `REQUIRE_DB=1` を注入する。
- **残してよいダブルは「実 DB が観測できない seam」の検証のみ**(SPEC-013 R2 例外。**in-memory fake を backing にしない**): (1) 実 DB で誘発できない障害系(未分類エラー → `writeError` fallback 等)の最小スタブ、(2) service → ポート振り分け(reader / writer)や narrow-port 型証明を検証する、**実 repo をラップした計数デコレータ**、(3) 永続化に到達しないことを示す nil-repo。それ以外の「DB を代替する in-memory fake」は禁止。
- **対象外(DB 非依存なので実 DB 化しない)**: domain 層の純ロジック(状態遷移・VO)、`cmd/*/env.go` の env → Config 写像(`t.Setenv`)、`persistence_selection_test.go`(`Config.DSN` / `Validate`)。
