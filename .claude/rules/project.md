# プロジェクト構成

cc-orchestrator は、Claude Code の subagent 群でソフトウェア開発ワークフロー全体を回すための monorepo。

## ディレクトリ

| パス | 役割 |
|---|---|
| `app/web` | フロントエンド (TypeScript / React) |
| `app/api` | バックエンド API (Go) |
| `app/auth` | 認証・認可 API (Go / OAuth 2.0 + OIDC)。`app/api` と同一の DDD レイヤ構成 |
| `app/iac` | インフラ (Terraform) |
| `docs/specs` | 機能仕様(Spec)。`spec` skill が固定テンプレートで管理。機能開発の起点となる一次情報 |
| `docs/issues` | 不具合・課題(Issue)。`issue` skill が固定テンプレートで管理。issue-creator agent が起票する |
| `docs/plans` | 実装計画。planner agent が作成 |

## 共通原則

- ドキュメント・Issue・計画・レビュー報告は日本語で書く。コード内の識別子・コメント・コミットメッセージは英語
- 仕様の一次情報は `docs/specs`。実装と仕様が食い違う場合は仕様を正とし、仕様側に問題があれば勝手に解釈せず Issue として起票して指摘する
- 各 stack の規約・コマンドは `.claude/rules/{web,api,iac}.md` に従う
- 秘密情報(API key・認証情報・アカウント ID)をコード・ドキュメント・tfvars に直接書かない
- 担当範囲外の stack のコードを変更しない(例: web の実装中に api を書き換えない)。範囲外の問題を見つけたら報告する
