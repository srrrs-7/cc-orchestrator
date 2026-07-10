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
| api | 標準 `go test` | table-driven test を基本形にする。外部依存(DB・外部 API)は interface 越しに fake へ差し替える |
| iac | `terraform validate` + `terraform plan` | plan 結果の差分が意図通りかを確認する。破壊的変更(replace)は必ず報告する |

## 永続化リポジトリのふるまい契約テスト(api / auth)

同一の `Repository` interface を複数実装(`infra/memory` / `infra/postgres`)が満たす Go stack では、**ふるまい契約テストを実装ごとに二重化しない**。ドメインが定義する `Repository` を単一の正とし、`infra/repotest` パッケージに `Run<集約>RepositoryContract(t, newRepo)` を 1 つだけ書いて各実装のテストから呼ぶ:

- `infra/memory/<集約>_repository_contract_test.go` — 既定(untagged)ビルドで memory 実装に対して回す
- `infra/postgres/<集約>_repository_integration_test.go` — `//go:build integration` タグ付きで実 DB に対して回す(`make test-integration`。実行契約の正は `.claude/rules/db.md`)
- `infra/repotest` は両ビルドから使うため、標準ライブラリと対象ドメインパッケージ以外に依存しない
