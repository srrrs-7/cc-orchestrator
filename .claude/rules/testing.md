---
paths:
  - "app/**"
---

# テスト規約

## 共通

- テストは実装と同じ Issue 内で必ず追加する。テストなしで Issue を done にしない
- テストは Issue の受け入れ条件と対応付ける(どのテストがどの条件を検証するかを明確に)
- 観点は 正常系 / 異常系 / 境界値 の 3 つを最低限カバーする
- 落ちているテストを skip・削除して「通した」ことにしない。落ちる理由を報告する
- 実行順序に依存するテスト、実時間 sleep に依存するテストを書かない

## stack ごと

| stack | フレームワーク | 方針 |
|---|---|---|
| web | Vitest + React Testing Library | ユーザー視点(role / label)でクエリする。実装詳細(class 名・内部 state)に依存しない |
| api | 標準 `go test` | table-driven test を基本形にする。外部依存(DB・外部 API)は interface 越しに fake へ差し替える |
| iac | `terraform validate` + `terraform plan` | plan 結果の差分が意図通りかを確認する。破壊的変更(replace)は必ず報告する |
