# プロジェクト構成

cc-orchestrator は、Claude Code の subagent 群でソフトウェア開発ワークフロー全体を回すための monorepo。

## ディレクトリ

| パス | 役割 |
|---|---|
| `app/web` | フロントエンド (TypeScript / React) |
| `app/api` | バックエンド API (Go) |
| `app/iac` | インフラ (Terraform) |
| `docs/specs` | 仕様書。全作業の起点となる一次情報 |
| `docs/issues` | Issue(作業単位)。issue-creator agent が作成 |
| `docs/plans` | 実装計画。planner agent が作成 |

## 共通原則

- ドキュメント・Issue・計画・レビュー報告は日本語で書く。コード内の識別子・コメント・コミットメッセージは英語
- 仕様の一次情報は `docs/specs`。実装と仕様が食い違う場合は仕様を正とし、仕様側に問題があれば勝手に解釈せず Issue として起票して指摘する
- 各 stack の規約・コマンドは `.claude/rules/{web,api,iac}.md` に従う
- 秘密情報(API key・認証情報・アカウント ID)をコード・ドキュメント・tfvars に直接書かない
- 担当範囲外の stack のコードを変更しない(例: web の実装中に api を書き換えない)。範囲外の問題を見つけたら報告する
