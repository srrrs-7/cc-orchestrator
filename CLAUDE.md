# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 開発体制: Multi-Agent 強制

すべての開発タスクは **admin**(メインセッションの Claude・最上位モデル)が細分化・計画し、`.claude/agents/` の subagent に割り振って実行する。admin は実装・テスト・チェック・レビューを直接行わない(軽微な修正も例外にしない)。役割定義・割り振り表・ホワイトリスト・禁止事項は `.claude/rules/orchestration.md`(常時ロード)に従うこと。

## リポジトリ概要

cc-orchestrator は、Claude Code の subagent 群でソフトウェア開発ワークフロー全体(Spec → 計画 → TDD → 実装 → チェック → レビュー → 記録)を回すための monorepo。中核は `.claude/` の agents / rules / skills 定義と `docs/` のドキュメント体系で、`app/` はそのワークフローで開発する題材。

実装状況(スナップショット。正確な現状は各ディレクトリを参照):

- `app/api` — Go の DDD サンプル(タスク管理)。`domain/task` + `service` + `infra/memory` + `infra/postgres` + `route` を実装済みで、実体のあるコードの中心。標準ライブラリ主体だが、永続化層 `infra/postgres` のみ Postgres ドライバ `pgx` に依存する(SPEC-005)
- `app/auth` — OAuth 2.0 + OIDC 認可サーバー(Go)。`app/api` と同じ DDD レイヤ構成で authorize / token / userinfo / discovery と JWT(RS256 / JWKS / PKCE S256)を実装済み(AUTH-001)。標準ライブラリ主体で、永続化層 `infra/postgres`(client / user / authcode)のみ `pgx` に依存する(SPEC-005)
- `app/web` — TypeScript / React。feature-sliced な SPA(`features/tasks` に domain / api / hooks / components、`shared/`、MSW モック、Vitest)を実装済み
- `app/iac` — Terraform。`modules/{network,db,platform,service,cdn}` と `envs/dev`(ルートモジュール)を実装済み(SPEC-001 / SPEC-004)。`platform` は共有基盤(ECS クラスタ / ALB 等)、`service` は 1 サービス分の定義で api・auth の 2 回インスタンス化する

- パイプラインの全フェーズと agent の役割分担: `.claude/rules/workflow.md`(常時ロード)
- ディレクトリ構成と共通原則: `.claude/rules/project.md`(常時ロード)

## 実装アーキテクチャ(app/api)

app/api は Evans の DDD レイヤ化アーキテクチャの Go サンプル。詳細は `app/api/README.md` が正。要点だけ:

- 依存の向きは一方向 `route → service → domain`。`domain` はどの層にも依存しない
- 永続化は `domain/task/repository.go` の `Repository` interface で抽象化し、`infra/memory` が実装する(依存性逆転)。DB 実装を足すときはこの interface を満たす形で `infra/` に追加する(実装例: `infra/postgres`。goose + sqlc、SPEC-005)
- `cmd/api/main.go` はコンポジションルート(配線のみ・ロジックを持たない)
- 集約ルート `Task` はフィールド非公開で、状態遷移は振る舞いメソッド(`Start` / `Complete` 等)経由のみ。ドメインエラーは sentinel / カスタム型 + `errors.Is` / `errors.As` で判定する
- `app/auth` も同一のレイヤ構成(`domain` / `service` / `infra` / `route` / `cmd`)を踏襲する(`.claude/rules/auth.md`)
- `app/web` も同じ「`domain` を最下層に置く」原則を適用する。ビジネスルールは React 非依存の純関数として `features/<feature>/domain/` に置き、依存方向は一方向 `components → hooks → (api | domain)`(`.claude/rules/web.md`)

## 型契約(app/api ⇄ app/web)

app/api(Go)と app/web(TypeScript)の request/response 型は **単一の OpenAPI 契約から生成**して一致させる(SPEC-003)。手書きで二重定義しない:

- 契約の正は `app/api/docs/openapi.yaml`。app/api の swag v2 注釈から `cd app/api && make openapi` で生成する(`make check` には含めない)
- app/web はこの契約から `cd app/web && bun run generate`(@hey-api/openapi-ts、設定は `app/web/openapi-ts.config.ts`)で型 / Zod スキーマ / TanStack Query クライアントを `src/features/tasks/api/generated/` に生成する(コミット対象。Biome の対象外だが typecheck / build は通す)
- Go の DTO を変えたら **両方を再生成して commit** する。「Go を変えたのに再生成し忘れ」は CI の `.github/workflows/contract-drift.yml`(Go + Bun 双方を要する唯一のジョブ)が検出して fail する。日常の checker は stack ごとの `make check` / `bun run` で分離されており、この drift 検査だけが跨り stack

## 永続化(app/api / app/auth)

app/api・app/auth のデータは Postgres に永続化する(SPEC-005)。DDD の依存性逆転を保ち、`domain/<集約>/repository.go` の `Repository` interface を `infra/postgres` が実装する(`infra/memory` と同格・切替可能)。スキーマは **goose** マイグレーション、クエリ→型安全 Go は **sqlc** で単一ソースから生成しコミットする(OpenAPI 契約と同じ思想):

- 規約・コマンド・レイアウト・接続 env 契約の正は `.claude/rules/db.md`。担当は impl-db agent
- 新規 runtime 依存は `pgx` のみ(goose / sqlc は `go run <pkg>@<ver>` の CLI で go.mod 非搭載)
- api ⇄ auth は同一 RDS の単一 database・別スキーマ(`search_path`)で分離。本番は Postgres 必須(接続情報なしは起動失敗 = fail-closed)、`infra/memory` フォールバックは local / test 限定
- sqlc の再生成漏れは CI の `.github/workflows/sqlc-drift.yml` が検出。マイグレーション健全性は `api-integration` / `auth-integration` job(postgres service)で検査

## ルールのロード構造

`.claude/rules/{web,api,auth,iac,db,testing}.md` は frontmatter の `paths` により、対象パス(`app/<stack>/**`)のファイルを扱うときだけ自動ロードされる。orchestrator として計画・委譲・コマンド実行を行うときは、対象 stack の rules を明示的に Read すること。各 rules の「コマンド」表は checker / tester が実行するコマンドの契約(例: `app/web/package.json` は表の scripts を必ず提供する)。

## コマンド早見表(正は各 rules ファイルの「コマンド」表)

| stack | 実行場所 | ツール |
|---|---|---|
| web | `app/web` | Bun(runtime / pm)で `bun run <script>` — `format:check` / `format`(Biome)/ `lint`(Biome)/ `typecheck`(tsgo、`tsc` ではない)/ `test`(Vitest + RTL)/ `build`(tsgo + Vite)。単一テストは `bun run test <path>` |
| api / auth | `app/api` / `app/auth` | make(実体は各 stack の `Makefile`)— `make check`(= fmt-check + lint + vet + build + test)、個別に `make fmt` / `fmt-check` / `lint` / `vet` / `build` / `test` / `test-race` / `run`。単一テストは `go test -run '^TestName$' ./path/...` |
| iac | `app/iac`(fmt)/ `envs/<env>`(validate 等) | make(実体は `app/iac/Makefile`、環境は `ENV=<env>` 既定 dev)— `make check`(= fmt-check + validate + lint + security)、個別に `make fmt` / `fmt-check` / `init-local` / `validate` / `lint` / `security` / `plan`。**fmt は `app/iac` ルート全体(modules + envs)、validate 以降は `envs/<env>` 基点**で実行する |

**`terraform apply` は実行しない。** plan の結果を報告し、apply の判断は必ずユーザーに委ねる。

永続化(DB)系ターゲット(`make sqlc` / `migrate-up` / `migrate-down` / `migrate-status` / `migrate-create` / `test-integration`)は各 Go スタックの Makefile が提供するが、生成 / スキーマ操作 / 実 DB 依存のため `make check` には含めない(正は `.claude/rules/db.md`)。

## ローカル実行(全スタック)

リポジトリルートの `Makefile` + `compose.yml` で 4 サービス(web / api / auth と `postgres`。各 app は `app/*/Dockerfile` をビルド)をまとめて起動する: `make up`(フォアグラウンド)/ `make up-d`(バックグラウンド)/ `make down` / `make logs` / `make ps`。api・auth は compose 上では Postgres 経路(fail-closed)のため、`make up` / `up-d` は先に `make migrate`(goose。**ホストの Go ツールチェーンを要する**)を実行してからサービスを起動する。`make db-up`(postgres のみ)/ `make migrate` も個別に使える。起動後は web `http://localhost:8080` / api `http://localhost:8081` / auth `http://localhost:8082` / postgres `127.0.0.1:5432`(ホストは `127.0.0.1` のみバインド)。全ターゲットは `make help`。

ルート `Makefile` には AWS デプロイ用の `push-images`(api/auth を ARM64 で ECR に build & push)/ `deploy-web`(web を build して S3 sync + CloudFront invalidation)もある(SPEC-004)。**これらは `terraform apply` 済み + AWS 認証情報を前提とする手動実行ターゲットで、agent は実行しない。**

## ドキュメント規約

- 機能仕様は `docs/specs/`、不具合・課題は `docs/issues/` に時系列で記録する(命名規則と読み方は各ディレクトリの README 参照)
- 仕様の作成・更新は `/spec`、課題の起票・更新は `/issue` スキルを必ず使う(直接ファイルを作らない)。テンプレートと更新手順は `.claude/skills/{spec,issue}/` が唯一の情報源
- PR の作成は `/github-pr` スキルを使う(本文は最小限の概要のみを固定テンプレートで記載)
- リリース PR は `/release-pr vX.Y.Z base=<branch>` スキルを使う(main HEAD から `vX.Y.Z` を切り、`base..main` の変更・ユーザー影響・関連 PR/Issue・インフラのデプロイ要件をテーブルで集約)
- ファイル名 `YYYYMMDD-NNN-<slug>.md` の連番 NNN が ID(SPEC-NNN / ISSUE-NNN)。採番は既存ファイルの連番最大値 +1
- 現状把握: 各ファイルの frontmatter の `status` と、「経緯」セクションの末尾が最新状態。経緯は追記のみで、過去エントリは編集しない
