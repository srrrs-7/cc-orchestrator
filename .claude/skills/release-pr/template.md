# vX.Y.Z

## 概要

<このリリースの要点。1〜2文>

## 変更一覧

| 変更 | ユーザー影響 | PR | Issue / Spec |
|---|---|---|---|
| <変更の要約> | <誰にどう影響するか。不明は「要確認」> | [#NN](PR_URL) | [ISSUE-NNN](docs/issues/...) / [SPEC-NNN](docs/specs/...) / — |

## インフラ(デプロイ要件)

`app/iac` に変更がある場合のみ記載。無ければ「なし」。

| 環境 / モジュール | 変更内容 | 必要作業 |
|---|---|---|
| `envs/<env>` / `modules/<module>` | <例: RDS モジュール追加> | terraform plan → 承認 → apply |

- `apply` はこの PR では実行しない。plan 結果を添え、実施可否はユーザーが判断する。

🤖 Generated with [Claude Code](https://claude.com/claude-code)
