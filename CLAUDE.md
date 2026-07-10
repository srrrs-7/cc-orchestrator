# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 開発体制: Multi-Agent 強制

すべての開発タスクは **admin**(メインセッションの Claude・最上位モデル)が細分化・計画し、`.claude/agents/` の subagent に割り振って実行する。admin は実装・テスト・チェック・レビューを直接行わない(軽微な修正も例外にしない)。役割定義・割り振り表・ホワイトリスト・禁止事項は `.claude/rules/orchestration.md`(常時ロード)に従うこと。

## リポジトリ概要

cc-orchestrator は、Claude Code の subagent 群でソフトウェア開発ワークフロー全体(Spec → 計画 → TDD → 実装 → チェック → レビュー → 記録)を回すための monorepo。中核は `.claude/` の agents / rules / skills 定義と `docs/` のドキュメント体系で、`app/` はそのワークフローで開発する題材。

実装状況(スナップショット。正確な現状は各ディレクトリを参照):

- `app/api` — Go の DDD サンプル(タスク管理)。`domain/task` + `service` + `infra/memory` + `infra/postgres` + `route` を実装済みで、実体のあるコードの中心。標準ライブラリ主体だが、永続化層 `infra/postgres` のみ Postgres ドライバ `pgx` に依存する(SPEC-005)
- `app/auth` — OAuth 2.0 + OIDC 認可サーバー(Go)。`app/api` と同じ DDD レイヤ構成で authorize / token / userinfo / discovery を実装済み(AUTH-001)。グラントは Authorization Code(PKCE S256)+ refresh_token(RFC 6749 §6、rotation + family 単位の reuse 検出。SPEC-006)、トークンは JWT(RS256 / JWKS)。ドメインは client / user / authcode / refreshtoken の 4 集約 +(署名ポートのみで永続化しない)token。標準ライブラリ主体で、永続化層 `infra/postgres`(client / user / authcode / refreshtoken)のみ `pgx` に依存する(SPEC-005)
- `app/web` — TypeScript / React。feature-sliced な SPA(`features/tasks` に domain / api / hooks / components、`shared/`、MSW モック、Vitest)を実装済み
- `app/iac` — Terraform。`modules/{network,db,platform,service,cdn}` と `envs/dev`(ルートモジュール)を実装済み(SPEC-001 / SPEC-004)。`platform` は共有基盤(ECS クラスタ / ALB 等)、`service` は 1 サービス分の定義で api・auth の 2 回インスタンス化する
- `app/migrator` — Go(独立モジュール)。DB マイグレーション実行ツール。`-target api|auth` で対象 DB を作成(冪等)+ 各スタックの `db/migrations` を goose 適用する。goose を api/auth から隔離するためのモジュール(SPEC-005)

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
- 新規 runtime 依存は `pgx` のみ。sqlc は `go run <pkg>@<ver>` の CLI(go.mod 非搭載)。**goose は専用モジュール `app/migrator` に閉じ、api/auth の `go.mod` には載せない**
- **api ⇄ auth は同一 RDS インスタンス上の別データベース(`api` / `auth`)で分離**(`search_path` は使わず `DB_NAME` を per-service で指定)。本番は Postgres 必須(接続情報なしは起動失敗 = fail-closed)、`infra/memory` フォールバックは local / test 限定
- **マイグレーションは `app/migrator`(単一 Go バイナリ / 単一イメージ)が `-target api|auth` で対象 DB を作成 + goose 適用**する。ローカルはルート `make migrate`、本番は ECS init コンテナ
- **CQRS read/write 分離(SPEC-010)**: `infra/postgres` は writer / reader の 2 プール(`OpenPair`)を持ち、ドメインは `Reader`(query)/ `Writer`(command)/ 合成 `Repository` にポート分割する。接続 env は writer 用 `DB_*` に加え reader 用 `DB_READER_*` を項目ごとに読み、**未設定項目は writer 値へフォールバック**(全項目未設定なら単一プールを共有し二重に開かない)。設計・env 契約・実装 seam の正は `.claude/rules/db.md`。担当はポートが impl-api/auth、2 プール実装が impl-db
- `infra/memory` と `infra/postgres` が同一 `Repository` を満たすことは、`infra/repotest` の**共有ふるまい契約テスト**(`Run<集約>RepositoryContract`)を両実装から回して保証する(テストロジックを実装ごとに二重化しない)。postgres 側は `integration` build tag 付きで実 DB に対して回す
- sqlc の再生成漏れは CI の `.github/workflows/sqlc-drift.yml` が検出。マイグレーション健全性は `api-integration` / `auth-integration` job(postgres service + migrator)で検査

## ルールのロード構造

`.claude/rules/{web,api,auth,iac,db,testing}.md` は frontmatter の `paths` により、対象パス(`app/<stack>/**`)のファイルを扱うときだけ自動ロードされる。orchestrator として計画・委譲・コマンド実行を行うときは、対象 stack の rules を明示的に Read すること。各 rules の「コマンド」表は checker / tester が実行するコマンドの契約(例: `app/web/package.json` は表の scripts を必ず提供する)。

## コマンド早見表(正は各 rules ファイルの「コマンド」表)

**実行環境(SPEC-009)**: 下表のコマンド(web の `bun run <script>`、api / auth / migrator / iac の `make`)は、ホストに go / bun / golangci-lint / terraform 等を入れず、**単一の toolchain コンテナ内で実行**される(ホストの前提は Docker のみ。サプライチェーン攻撃対策)。各 Makefile と `bin/bun` が透過ラッパーとしてホスト呼び出しを `docker compose -f compose.tools.yml run` へ委譲するため、**コマンド名・契約は不変**。通常の検査系は `--network none`(オフライン)、依存取得・DB 到達フェーズのみネットワーク有効。詳細は `docs/specs/20260710-009-containerized-toolchain-no-host-runtime.md`。

| stack | 実行場所 | ツール |
|---|---|---|
| web | `app/web` | Bun(runtime / pm)で `bun run <script>` — `format:check` / `format`(Biome)/ `lint`(Biome)/ `typecheck`(tsc、TypeScript 7 ネイティブコンパイラ)/ `test`(Vitest + RTL)/ `build`(tsc + Vite)。単一テストは `bun run test <path>` |
| api / auth | `app/api` / `app/auth` | make(実体は各 stack の `Makefile`)— `make check`(= fmt-check + lint + vet + build + test)、個別に `make fmt` / `fmt-check` / `lint` / `vet` / `build` / `test` / `test-race` / `run`。単一テストは `go test -run '^TestName$' ./path/...` |
| iac | `app/iac`(fmt)/ `envs/<env>`(validate 等) | make(実体は `app/iac/Makefile`、環境は `ENV=<env>` 既定 dev)— `make check`(= fmt-check + validate + lint + security)、個別に `make fmt` / `fmt-check` / `init-local` / `validate` / `lint` / `security` / `plan`。**fmt は `app/iac` ルート全体(modules + envs)、validate 以降は `envs/<env>` 基点**で実行する |

**`terraform apply` は実行しない。** plan の結果を報告し、apply の判断は必ずユーザーに委ねる。

永続化(DB)系ターゲット(各 Go スタックの `make sqlc` / `migrate-create` / `test-integration`、およびルートの `make migrate`= `app/migrator` 実行)は、生成 / DB 作成・マイグレーション / 実 DB 依存のため `make check` には含めない(正は `.claude/rules/db.md`)。**マイグレーション実行は `app/migrator`(`-target api|auth`)に集約**されている(per-stack の `migrate-up/down/status` は廃止)。

## ローカル実行(全スタック)

リポジトリルートの `Makefile` + `compose.yml` で 4 サービス(web / api / auth と `postgres`。各 app は `app/*/Dockerfile` をビルド)をまとめて起動する: `make up`(フォアグラウンド)/ `make up-d`(バックグラウンド)/ `make down` / `make logs` / `make ps`。api・auth は compose 上では Postgres 経路(fail-closed)のため、`make up` / `up-d` は先に `make migrate`(`app/migrator` 経由で api/auth の DB 作成 + マイグレーション。**SPEC-009 により toolchain コンテナ内で実行**され、ホストに Go は不要=前提は Docker のみ)を実行してからサービスを起動する。`make db-up`(postgres のみ)/ `make migrate` も個別に使える。起動後は web `http://localhost:8080` / api `http://localhost:8081` / auth `http://localhost:8082` / postgres `127.0.0.1:5432`(ホストは `127.0.0.1` のみバインド)。全ターゲットは `make help`。

ルート `Makefile` には AWS デプロイ用の `push-images`(api/auth を ARM64 で ECR に build & push)/ `deploy-web`(web を build して S3 sync + CloudFront invalidation)もある(SPEC-004)。**これらは `terraform apply` 済み + AWS 認証情報を前提とする手動実行ターゲットで、agent は実行しない。**

## CI(GitHub Actions)

CI/CD は 4 つの workflow に分かれる(担当は impl-ci)。上表の `make` / `bun run` 契約をコンテナ内で回すのは以下:

- **`cicd.yml`**(push / PR): メイン CI。先頭の `changes` job(`dorny/paths-filter`)で変更 stack を検出し、**変更のあった stack の job だけを起動**する(web / api / auth / migrator / iac の各 `check` job と、DB 依存の `api-integration` / `auth-integration` / `migrator-integration` job)。「特定 stack の CI が走らない」ときはまずこの path-filter を疑う(workflow 自身の変更は全 job を再実行させる)
- **`contract-drift.yml`**(push / PR): OpenAPI 契約の再生成漏れ検査。**Go + Bun 双方を要する唯一のジョブ**(日常の checker は stack ごとに分離)
- **`sqlc-drift.yml`**(push / PR): sqlc の再生成漏れ検査(api / auth。自前の path-filter を持つ)
- **`deploy.yml`**: `workflow_dispatch` 専用の**手動**デプロイ(`build-web` / `push-images` / `deploy-web`)。push / PR では走らない

## ドキュメント規約

- 機能仕様は `docs/specs/`、不具合・課題は `docs/issues/` に時系列で記録する(命名規則と読み方は各ディレクトリの README 参照)
- 仕様の作成・更新は `/spec`、課題の起票・更新は `/issue` スキルを必ず使う(直接ファイルを作らない)。テンプレートと更新手順は `.claude/skills/{spec,issue}/` が唯一の情報源
- PR の作成は `/github-pr` スキルを使う(本文は最小限の概要のみを固定テンプレートで記載)
- リリース PR は `/release-pr vX.Y.Z base=<branch>` スキルを使う(main HEAD から `vX.Y.Z` を切り、`base..main` の変更・ユーザー影響・関連 PR/Issue・インフラのデプロイ要件をテーブルで集約)
- ファイル名 `YYYYMMDD-NNN-<slug>.md` の連番 NNN が ID(SPEC-NNN / ISSUE-NNN)。採番は既存ファイルの連番最大値 +1
- 現状把握: 各ファイルの frontmatter の `status` と、「経緯」セクションの末尾が最新状態。経緯は追記のみで、過去エントリは編集しない
