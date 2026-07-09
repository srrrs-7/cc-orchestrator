# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 開発体制: Multi-Agent 強制

すべての開発タスクは **admin**(メインセッションの Claude・最上位モデル)が細分化・計画し、`.claude/agents/` の subagent に割り振って実行する。admin は実装・テスト・チェック・レビューを直接行わない(軽微な修正も例外にしない)。役割定義・割り振り表・ホワイトリスト・禁止事項は `.claude/rules/orchestration.md`(常時ロード)に従うこと。

## リポジトリ概要

cc-orchestrator は、Claude Code の subagent 群でソフトウェア開発ワークフロー全体(Spec → 計画 → TDD → 実装 → チェック → レビュー → 記録)を回すための monorepo。中核は `.claude/` の agents / rules / skills 定義と `docs/` のドキュメント体系で、`app/` はそのワークフローで開発する題材。

実装状況(スナップショット。正確な現状は各ディレクトリを参照):

- `app/api` — Go(標準ライブラリのみ)の DDD サンプル(タスク管理)。`domain/task` + `service` + `infra/memory` + `route` を実装済みで、実体のあるコードの中心
- `app/auth` — OAuth 2.0 + OIDC 認可サーバー(Go / 標準ライブラリのみ)。`app/api` と同じ DDD レイヤ構成で authorize / token / userinfo / discovery と JWT(RS256 / JWKS / PKCE S256)を実装済み(AUTH-001)
- `app/web` — TypeScript / React。feature-sliced な SPA(`features/tasks` に domain / api / hooks / components、`shared/`、MSW モック、Vitest)を実装済み
- `app/iac` — Terraform。`modules/{network,db,app,cdn}` と `envs/dev`(ルートモジュール)を実装済み(SPEC-001)

- パイプラインの全フェーズと agent の役割分担: `.claude/rules/workflow.md`(常時ロード)
- ディレクトリ構成と共通原則: `.claude/rules/project.md`(常時ロード)

## 実装アーキテクチャ(app/api)

app/api は Evans の DDD レイヤ化アーキテクチャの Go サンプル。詳細は `app/api/README.md` が正。要点だけ:

- 依存の向きは一方向 `route → service → domain`。`domain` はどの層にも依存しない
- 永続化は `domain/task/repository.go` の `Repository` interface で抽象化し、`infra/memory` が実装する(依存性逆転)。DB 実装を足すときはこの interface を満たす形で `infra/` に追加する
- `cmd/api/main.go` はコンポジションルート(配線のみ・ロジックを持たない)
- 集約ルート `Task` はフィールド非公開で、状態遷移は振る舞いメソッド(`Start` / `Complete` 等)経由のみ。ドメインエラーは sentinel / カスタム型 + `errors.Is` / `errors.As` で判定する
- `app/auth` も同一のレイヤ構成(`domain` / `service` / `infra` / `route` / `cmd`)を踏襲する(`.claude/rules/auth.md`)
- `app/web` も同じ「`domain` を最下層に置く」原則を適用する。ビジネスルールは React 非依存の純関数として `features/<feature>/domain/` に置き、依存方向は一方向 `components → hooks → (api | domain)`(`.claude/rules/web.md`)

## 型契約(app/api ⇄ app/web)

app/api(Go)と app/web(TypeScript)の request/response 型は **単一の OpenAPI 契約から生成**して一致させる(SPEC-003)。手書きで二重定義しない:

- 契約の正は `app/api/docs/openapi.yaml`。app/api の swag v2 注釈から `cd app/api && make openapi` で生成する(`make check` には含めない)
- app/web はこの契約から `cd app/web && bun run generate`(@hey-api/openapi-ts、設定は `app/web/openapi-ts.config.ts`)で型 / Zod スキーマ / TanStack Query クライアントを `src/features/tasks/api/generated/` に生成する(コミット対象。Biome の対象外だが typecheck / build は通す)
- Go の DTO を変えたら **両方を再生成して commit** する。「Go を変えたのに再生成し忘れ」は CI の `.github/workflows/contract-drift.yml`(Go + Bun 双方を要する唯一のジョブ)が検出して fail する。日常の checker は stack ごとの `make check` / `bun run` で分離されており、この drift 検査だけが跨り stack

## ルールのロード構造

`.claude/rules/{web,api,auth,iac,testing}.md` は frontmatter の `paths` により、対象パス(`app/<stack>/**`)のファイルを扱うときだけ自動ロードされる。orchestrator として計画・委譲・コマンド実行を行うときは、対象 stack の rules を明示的に Read すること。各 rules の「コマンド」表は checker / tester が実行するコマンドの契約(例: `app/web/package.json` は表の scripts を必ず提供する)。

## コマンド早見表(正は各 rules ファイルの「コマンド」表)

| stack | 実行場所 | ツール |
|---|---|---|
| web | `app/web` | Bun(runtime / pm)で `bun run <script>` — `format:check` / `format`(Biome)/ `lint`(Biome)/ `typecheck`(tsgo、`tsc` ではない)/ `test`(Vitest + RTL)/ `build`(tsgo + Vite)。単一テストは `bun run test <path>` |
| api / auth | `app/api` / `app/auth` | make(実体は各 stack の `Makefile`)— `make check`(= fmt-check + lint + vet + build + test)、個別に `make fmt` / `fmt-check` / `lint` / `vet` / `build` / `test` / `test-race` / `run`。単一テストは `go test -run '^TestName$' ./path/...` |
| iac | `app/iac/envs/<env>` | `terraform fmt` / `terraform validate` / `tflint --recursive` / `trivy config .` / `terraform plan` |

**`terraform apply` は実行しない。** plan の結果を報告し、apply の判断は必ずユーザーに委ねる。

## ローカル実行(全スタック)

リポジトリルートの `Makefile` + `compose.yml` で 3 サービス(各 `app/*/Dockerfile` をビルド)をまとめて起動する: `make up`(フォアグラウンド)/ `make up-d`(バックグラウンド)/ `make down` / `make logs` / `make ps`。起動後は web `http://localhost:8080` / api `http://localhost:8081` / auth `http://localhost:8082`(ホストは `127.0.0.1` のみバインド)。全ターゲットは `make help`。

## ドキュメント規約

- 機能仕様は `docs/specs/`、不具合・課題は `docs/issues/` に時系列で記録する(命名規則と読み方は各ディレクトリの README 参照)
- 仕様の作成・更新は `/spec`、課題の起票・更新は `/issue` スキルを必ず使う(直接ファイルを作らない)。テンプレートと更新手順は `.claude/skills/{spec,issue}/` が唯一の情報源
- PR の作成は `/github-pr` スキルを使う(本文は最小限の概要のみを固定テンプレートで記載)
- リリース PR は `/release-pr vX.Y.Z base=<branch>` スキルを使う(main HEAD から `vX.Y.Z` を切り、`base..main` の変更・ユーザー影響・関連 PR/Issue・インフラのデプロイ要件をテーブルで集約)
- ファイル名 `YYYYMMDD-NNN-<slug>.md` の連番 NNN が ID(SPEC-NNN / ISSUE-NNN)。採番は既存ファイルの連番最大値 +1
- 現状把握: 各ファイルの frontmatter の `status` と、「経緯」セクションの末尾が最新状態。経緯は追記のみで、過去エントリは編集しない
