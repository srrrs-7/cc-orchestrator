# cc-orchestrator

Claude Code subagent 群で Spec → 計画 → TDD → 実装 → チェック → レビュー → 記録の開発ワークフローを回す monorepo。`app/` に api / auth / web / iac / migrator、`docs/` に Spec / Issue / 計画、`.claude/` に agent / rules / skills 定義がある。

## 前提

- **Docker のみ**(ホストに go / bun / terraform 等は不要。SPEC-009)
- 詳細な開発規約・コマンド契約: [`CLAUDE.md`](CLAUDE.md)

## クイックスタート

```bash
# git hooks(コミット前 CI 相当チェック)を有効化 — 各 clone で 1 回
make setup-hooks

# ローカル Postgres + マイグレーション + 全サービス起動
make up-d
# web http://localhost:8080 / api http://localhost:8081 / auth http://localhost:8082
```

## Git hooks

pre-commit で、**ステージ済み変更**に応じた stack の `make check` と contract / sqlc drift 検査を toolchain コンテナ内で実行する(CI の path filter と同型。integration テストは含まない)。

| コマンド | 説明 |
|---|---|
| `make setup-hooks` | `core.hooksPath=.githooks` を設定 |
| `make hook-check` | pre-commit と同じ検証を手動実行 |
| `git commit --no-verify` | 1 回だけ hook をスキップ |

- **ホスト**: Docker 経由で toolbox コンテナ内に再実行
- **devcontainer**: `IN_TOOLBOX=1` のセッション内で直接実行

詳細: [`.githooks/README.md`](.githooks/README.md)

## よく使うコマンド

```bash
make help              # ルート Makefile のターゲット一覧
make migrate           # api/auth DB 作成 + goose 適用
make api-check         # app/api の make check(他 stack も auth-/web-/iac-/migrator- 接頭辞)
```
