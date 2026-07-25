# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 応答フォーマット: 要点のみ・最小文字数(必須)

会話でユーザーに返す出力は、**要点だけ**を**箇条書き**で書く。目的は (1) トークン使用量の削減、(2) 人間が読む文字量の削減。**網羅性より短さを優先する**(網羅的な記録は `docs/` とコード内コメントが担う)。

- **結論を先に 1 行** → 続けて箇条書き 3〜7 点。前置き・依頼文の復唱・謝辞・感想・自己評価は書かない
- **1 行 1 事実**。1 行は 100 文字程度まで。ネストは 1 段まで。同じことを言い換えない
- **差分だけ書く**: 「どのファイルを・何にしたか」。変更していない箇所・自明な背景・検討の過程は書かない
- **コードは貼らない**。`path:line` で位置を示す(数行で結論が変わる差分のみ例外)
- **表は 3 列以内・5 行以内**。長い理由や設計判断は本文に展開せず、コード内コメント / `docs/specs` / `docs/issues` に置いてそこを指す
- **検証は結論のみ**(例: `5 サービス healthy / 4 endpoint 200 / guard 3 本 fail 確認`)。ログ貼り付けは**失敗時の該当行だけ**
- **未完了・失敗・リスクは 1 行ずつ必ず出す**。短くすることと省いて隠すことは別
- 長文にするのは、ユーザーが「詳しく」「全文」「理由を説明して」と明示的に求めたときだけ

subagent の報告も同じ規約に従う(admin が検収時に長い報告を要約して返すのではなく、**報告自体を要点のみにさせる**)。

## 開発体制: Multi-Agent 強制

すべての開発タスクは **admin**(メインセッションの Claude・最上位モデル)が細分化・計画し、`.claude/agents/` の subagent に割り振って実行する。admin は実装・テスト・チェック・レビューを直接行わない(軽微な修正も例外にしない)。役割定義・割り振り表・ホワイトリスト・禁止事項は `.claude/rules/orchestration.md`(常時ロード)に従うこと。

## 自己改善(振り返り / retro)

`.claude/`(agents / rules / skills / CLAUDE.md)の精度を、実行中に遭遇した摩擦の記録から継続的に上げる仕組みを持つ。**product(app/*)の不具合は `docs/issues`、orchestration 自体の課題は `.claude/retro/`** に分ける。ループの正は `.claude/retro/README.md`:

- **記録**: admin が subagent の報告を検収する際、`.claude/` に起因する摩擦(ルールの曖昧さ・コマンド表と Makefile のズレ・担当範囲の穴・報告形式の不足等)を `retro` skill(`/retro`)で `.claude/retro/entries/`(RETRO-NNN、1 件 1 ファイル・追記のみ)に記録する
- **統括**: 溜まった記録を `retro-synthesizer` agent が横断分析し、頻度 × 深刻度で優先度付けした `.claude/` 改善提案レポート(RETROSUM-NNN)を `.claude/retro/reports/` に出す(**提案のみ**・直接編集しない)
- **適用**: admin が提案を検収して `.claude/` に適用し、解消した entry を `/retro` で `addressed` にする。記録 → 統括 → 適用の分離は review → impl の分離と同型

## リポジトリ概要

cc-orchestrator は、Claude Code の subagent 群でソフトウェア開発ワークフロー全体(Spec → 計画 → TDD → 実装 → チェック → レビュー → 記録)を回すための monorepo。中核は `.claude/` の agents / rules / skills 定義と `docs/` のドキュメント体系で、`app/` はそのワークフローで開発する題材。

実装状況(スナップショット。正確な現状は各ディレクトリを参照):

- `app/api` — Go の DDD サンプル(タスク管理)。`domain/task` + `service` + `infra/postgres` + `route` を実装済みで、実体のあるコードの中心。標準ライブラリ主体だが、永続化層 `infra/postgres` のみ Postgres ドライバ `pgx` に依存する(SPEC-005 / SPEC-011)。SPEC-015 で OIDC リソースサーバー化: `route/auth_middleware.go` が Bearer JWT(RS256)を auth の JWKS で検証する(`AUTH_ISSUER` / `AUTH_JWKS_URL` / `AUTH_AUDIENCE` env。未設定なら検証無効 = auth なしの単体開発可)
- `app/auth` — OAuth 2.0 + OIDC 認可サーバー(Go)。`app/api` と同じ DDD レイヤ構成(エントリポイントは `cmd/authz`)。エンドポイントは authorize / token / userinfo / discovery(openid-configuration / jwks.json)に加え、IdP の login / consent UI(cookie セッション + 永続化する scope 同意)・RP-Initiated Logout(`/logout`)・revoke(RFC 7009)・introspect(RFC 7662)・admin API(client / user 作成。API key 未設定なら経路ごと未登録 = fail-closed)を実装済み(基盤の経緯は `docs/plans/AUTH-001-plan.md`、機能拡張の傘は SPEC-015)。グラントは Authorization Code(PKCE S256)+ refresh_token(rotation + family 単位の reuse 検出。SPEC-006)。confidential client(client secret 認証)・API リソースサーバー向け audience 分離・`prompt`(none / login)/ `max_age` に対応。トークンは JWT(RS256 / JWKS)で、署名鍵は multi-key リングを `.secrets/auth-signing-keys.json` に永続化・ローテーションできる(`cmd/keygen`。ルート `make auth-signing-keys` / `rotate-auth-signing-keys`。未設定時は ephemeral 鍵)。ドメインは client / user / authcode / refreshtoken / consent / idpsession の 6 集約 +(署名ポートのみで永続化しない)token。標準ライブラリ主体で、永続化層 `infra/postgres` のみ `pgx` に依存し(SPEC-005)、idpsession のみ意図的に in-memory(`infra/memory`)
- `app/web` — TypeScript / React。feature-sliced な SPA(`features/{tasks,auth}` に domain / api / hooks / components、`app/`(シェル + TanStack Router)、`shared/`、MSW モック、Vitest)を実装済み。`features/auth` は OIDC RP(SPEC-015): Authorization Code + PKCE S256、トークンは sessionStorage(zod 検証)、期限前に単一飛行で silent refresh、Bearer は tasks の api client が注入し、ルート `/` / `/tasks/$taskId` は `beforeLoad` で認証ガード(`/login` / `/callback` あり)。スタイルは **Tailwind CSS v4**(`@tailwindcss/vite` + `src/index.css` の `@import "tailwindcss"`)、UI は **任意のディスプレイ幅で崩れないレスポンシブが必須要件**(SPEC-012。固定幅禁止・受け入れ幅などの正は `.claude/rules/web.md`)。CSP は HTTP ヘッダーで配信する(ローカルは nginx、AWS は CloudFront response headers policy。SPEC-014)
- `app/auth-web` — TypeScript / React。auth の admin API(client / user の管理)を操作する IdP 管理 UI。`app/web` と同一のツールチェーン(Bun / Biome / tsc / Vitest / Tailwind CSS v4 / TanStack Router・Query / MSW)・feature-sliced 構成(`features/admin`)・`Makefile` 契約を採り、nginx が `/auth/` を auth へ同一オリジン proxy する。OpenAPI 生成はなく admin API の型は zod スキーマで定義する。規約は `.claude/rules/web.md`(web と共通)、担当は impl-web
- `app/iac` — Terraform。`modules/{network,db,platform,service,cdn}` と `envs/dev`(ルートモジュール)を実装済み(SPEC-001 / SPEC-004)。`platform` は共有基盤(ECS クラスタ / ALB 等)、`service` は 1 サービス分の定義で api・auth の 2 回インスタンス化する。auth は署名鍵がプロセス毎のため desired_count=1 固定(`envs/dev/variables.tf`)
- `app/migrator` — Go(独立モジュール)。DB マイグレーション実行ツール。`-target api|auth` で対象 DB を作成(冪等)+ 各スタックの `infra/postgres/schema/migrations` を goose 適用する。goose を api/auth から隔離するためのモジュール(SPEC-005)

- パイプラインの全フェーズと agent の役割分担: `.claude/rules/workflow.md`(常時ロード)
- ディレクトリ構成と共通原則: `.claude/rules/project.md`(常時ロード)

## 実装アーキテクチャ(app/api)

app/api は Evans の DDD レイヤ化アーキテクチャの Go サンプル。詳細は `app/api/README.md` が正。要点だけ:

- 依存の向きは一方向 `route → service → domain`。`domain` はどの層にも依存しない
- 永続化は `domain/task/repository.go` の `Repository` interface で抽象化し、`infra/postgres` が実装する(依存性逆転。SPEC-011 で Postgres 一本化)。store を差し替える場合は `infra/postgres` を新実装で置換し、同じ `Run<集約>RepositoryContract` を回して振る舞い等価を確認する
- `cmd/api/main.go` はコンポジションルート(配線のみ・ロジックを持たない)
- 集約ルート `Task` はフィールド非公開で、状態遷移は振る舞いメソッド(`Start` / `Complete` 等)経由のみ。ドメインエラーは sentinel / カスタム型 + `errors.Is` / `errors.As` で判定する
- `app/auth` も同一のレイヤ構成(`domain` / `service` / `infra` / `route` / `cmd`)を踏襲する(`.claude/rules/auth.md`)
- `app/web` も同じ「`domain` を最下層に置く」原則を適用する。ビジネスルールは React 非依存の純関数として `features/<feature>/domain/` に置き、依存方向は一方向 `components → hooks → (api | domain)`(`.claude/rules/web.md`)

## 型契約(app/api ⇄ app/web)

app/api(Go)と app/web(TypeScript)の request/response 型は **単一の OpenAPI 契約から生成**して一致させる(SPEC-003)。手書きで二重定義しない:

- 契約の正は `app/api/docs/openapi.yaml`。app/api の swag v2 注釈から `cd app/api && make openapi` で生成する(`make check` には含めない)
- app/web はこの契約から `make -C app/web generate`(@hey-api/openapi-ts、設定は `app/web/openapi-ts.config.ts`)で型 / Zod スキーマ / TanStack Query クライアントを `src/features/tasks/api/generated/` に生成する(コミット対象。Biome の対象外だが typecheck / build は通す)
- Go の DTO を変えたら **両方を再生成して commit** する。「Go を変えたのに再生成し忘れ」は CI の `.github/workflows/contract-drift.yml`(Go + web 双方を要する唯一のジョブ)が検出して fail する。日常の checker は stack ごとの `make check` で分離されており、この drift 検査だけが跨り stack

## 永続化(app/api / app/auth)

app/api・app/auth のデータは Postgres に永続化する(SPEC-005 / SPEC-011)。DDD の依存性逆転を保ち、`domain/<集約>/repository.go` の `Repository` interface を `infra/postgres` が実装する。スキーマは **goose** マイグレーション、クエリ→型安全 Go は **sqlc** で単一ソースから生成しコミットする(OpenAPI 契約と同じ思想):

- 規約・コマンド・レイアウト・接続 env 契約の正は `.claude/rules/db.md`。担当は impl-db agent
- 新規 runtime 依存は `pgx` のみ。sqlc は `go run <pkg>@<ver>` の CLI(go.mod 非搭載)。**goose は専用モジュール `app/migrator` に閉じ、api/auth の `go.mod` には載せない**
- **api ⇄ auth は同一 RDS インスタンス上の別データベース(`api` / `auth`)で分離**(`search_path` は使わず `DB_NAME` を per-service で指定)。Postgres 必須(接続情報なしは起動失敗 = fail-closed。in-memory フォールバックは廃止、SPEC-011)。`DB_SSLMODE` の既定は `require`(fail-closed。ローカル非 TLS は明示的に `disable`)
- **マイグレーションは `app/migrator`(単一 Go バイナリ / 単一イメージ)が `-target api|auth` で対象 DB を作成 + goose 適用**する。ローカルはルート `make migrate`、本番は ECS init コンテナ
- **CQRS read/write 分離(SPEC-010)**: `infra/postgres` は writer / reader の 2 プール(`OpenPair`)を持ち、ドメインは `Reader`(query)/ `Writer`(command)/ 合成 `Repository` にポート分割する。接続 env は writer 用 `DB_*` に加え reader 用 `DB_READER_*` を項目ごとに読み、**未設定項目は writer 値へフォールバック**(全項目未設定なら単一プールを共有し二重に開かない)。設計・env 契約・実装 seam の正は `.claude/rules/db.md`。担当はポートが impl-api/auth、2 プール実装が impl-db
- `infra/repotest` の**共有ふるまい契約テスト**(`Run<集約>RepositoryContract`)を Postgres 実装(実 test DB。`integration` build tag は SPEC-013 で廃止し default `make test` に統合)に対して回し、store 差し替え時も同じ contract で振る舞い等価を確認する(テストロジックを実装ごとに二重化しない)
- sqlc の再生成漏れは CI の `.github/workflows/sqlc-drift.yml` が検出。DB 依存テストとマイグレーション健全性は `api` / `auth` の `check` job(postgres service + `make migrate-test` で `api_test` / `auth_test` を用意し `REQUIRE_DB=1` で実行)で検査する(SPEC-013 で旧 `api-integration` / `auth-integration` job を統合)

## ルールのロード構造

`.claude/rules/{web,api,auth,iac,db,testing}.md` は frontmatter の `paths` により、対象パス(`app/<stack>/**`)のファイルを扱うときだけ自動ロードされる。orchestrator として計画・委譲・コマンド実行を行うときは、対象 stack の rules を明示的に Read すること。各 rules の「コマンド」表は checker / tester が実行するコマンドの契約(例: `app/web/package.json` は表の scripts を必ず提供する)。

## コマンド早見表(正は各 rules ファイルの「コマンド」表)

**実行環境(SPEC-009)**: 下表のコマンドは、ホストに go / bun / golangci-lint / terraform 等を入れず、**単一の toolchain コンテナ内でのみ実行**される(ホストの前提は Docker のみ。サプライチェーン攻撃対策)。各 stack の `Makefile` がホスト呼び出しを `docker compose -f .devcontainer/compose.tools.yml run` へ委譲する。**ホストから bun / go を直接実行しない**。通常の検査系は `--network none`(オフライン)、依存取得・DB 到達フェーズのみネットワーク有効。詳細は `docs/specs/20260710-009-containerized-toolchain-no-host-runtime.md`。アプリの**起動(runtime)はこの toolchain レーンではなく別レーン**で行う(次節「実行境界: runtime はコンテナのみ」)。

| stack | 実行場所 | ツール |
|---|---|---|
| web / auth-web | `app/web` / `app/auth-web` | make(実体は各 stack の `Makefile`。契約は同一)— `make check`(= format-check + lint + typecheck + test + build)、個別に `make format-check` / `format` / `lint` / `typecheck` / `test` / `build` / `install`(`generate` = OpenAPI 型生成は web のみ)。単一テストは `make test TEST=<path>` |
| api / auth | `app/api` / `app/auth` | make(実体は各 stack の `Makefile`)— `make check`(= fmt-check + lint + vet + build + test)、個別に `make fmt` / `fmt-check` / `lint` / `vet` / `build` / `test` / `test-race` / `run`。単一テストは `go test -run '^TestName$' ./path/...` |
| iac | `app/iac`(fmt)/ `envs/<env>`(validate 等) | make(実体は `app/iac/Makefile`、環境は `ENV=<env>` 既定 dev)— `make check`(= fmt-check + validate + lint + security)、個別に `make fmt` / `fmt-check` / `init-local` / `validate` / `lint` / `security` / `plan`。**fmt は `app/iac` ルート全体(modules + envs)、validate 以降は `envs/<env>` 基点**で実行する |

**`terraform apply` は実行しない。** plan の結果を報告し、apply の判断は必ずユーザーに委ねる。

永続化(DB)系の生成・スキーマ操作ターゲット(各 Go スタックの `make sqlc` / `migrate-create`、およびルートの `make migrate` / `make migrate-test`= `app/migrator` 実行)は、生成 / DB 作成・マイグレーションのため `make check` には含めない(正は `.claude/rules/db.md`)。**一方、DB 依存テストは `make test` = `make check` の一部**(SPEC-013。実 test DB `api_test` / `auth_test` を要し、正規経路は `make migrate-test` で用意してから `REQUIRE_DB=1` で実行する。ルート `make test-db` はこの一連=migrate-test + api / auth の test を一括実行する)。テスト規約(TDD・`REQUIRE_DB=1` = fail / 未設定 = skip の意味論・doubles の可否)の正は `.claude/rules/testing.md`。**マイグレーション実行は `app/migrator`(`-target api|auth`)に集約**されている(per-stack の `migrate-up/down/status` は廃止)。

## 実行境界: runtime はコンテナのみ / ホスト実行禁止

コードが動く経路は **2 レーンだけ**で、混ぜない。ホストに要求するのは **Docker のみ**(go / bun / nginx / postgres をホストに入れない)。

| レーン | 何を動かすか | 実体 | 権限・マウント |
|---|---|---|---|
| **toolchain レーン**(SPEC-009) | 検査・ビルド・生成・依存解決(`make check` / `test` / `lint` / `build` / `sqlc` / `openapi` / `tidy`)と、リポジトリへ書き込む生成 CLI(`cmd/keygen` = `make auth-signing-keys`) | `.devcontainer/compose.tools.yml` の `tools` / `tools-offline` / `tools-db` | リポジトリを `/workspace` に read/write bind mount。`cap_drop: ALL` + `no-new-privileges`。ホスト資格情報(`~/.aws` / `~/.ssh` / `~/.npmrc`)と docker.sock は渡さない。通常は `--network none`、依存取得・DB 到達フェーズのみネットワーク有効 |
| **runtime レーン**(SPEC-016) | アプリの常駐プロセス(api / auth / web / auth-web / postgres)と、DB マイグレーション(`app/migrator`) | ローカルは `compose.yml`(各 `app/*/Dockerfile` の runtime イメージ。`migrator` は profile で `up` から除外された一発実行)、本番は ECS Fargate(`app/iac/modules/service` の app + migration init コンテナ) | 最小権限(後述)。マウントは postgres の `pgdata` と auth の署名鍵ファイル(read_only bind)のみ。リポジトリ・docker.sock・資格情報は渡さない |

### 禁止事項(必須)

- **ホストで runtime を起動しない。** `go run ./cmd/api` / `go run ./cmd/authz` / `bun run dev`(Vite dev server)/ ホストの nginx・postgres による代替起動はすべて禁止。ホストに Go / Bun が無い前提(SPEC-009)であるうえ、あったとしてもホストユーザーの全権限とファイルシステムを常駐プロセスに与えることになる
- **toolchain コンテナで runtime を常駐させない。** toolbox はビルドツール一式 + リポジトリの read/write bind mount を抱えている = runtime コンテナが手放しているはずの blast radius をそのまま持つ。`docker compose run tools` がポートを publish しないのはこの区別のため。`IN_TOOLBOX=1` の中で `make run` を叩くと各 stack Makefile のガードが明示エラーで落とす
- **runtime コンテナにマウントを増やさない。** リポジトリの bind mount / docker.sock / ホスト資格情報を渡さない。どうしても必要なら「単一ファイル + `read_only: true`」に限る(現状の唯一の例外が auth の署名鍵 `.secrets/auth-signing-keys.json`。既定では無効=コメントアウト)
- **runtime イメージにビルドツール・シェルを残さない。** Go 3 サービスは distroless static、web / auth-web は nginx + `dist` の静的ファイルのみ(Node / Bun / パッケージマネージャを含めない)

### 正規経路

| やりたいこと | コマンド |
|---|---|
| 全 5 サービスを起動 | ルート `make up`(フォアグラウンド)/ `make up-d`(デタッチ)。前段で `make migrate` が走る |
| 単一サービスだけ起動 | `make -C app/api run` / `make -C app/auth run`(ルートからは `make api-run` / `make auth-run`)= compose の該当サービスを `--build` して起動する。**ホスト `go run` ではない** |
| postgres だけ起動 | ルート `make db-up` |
| マイグレーション適用 | ルート `make migrate`(dev DB)/ `make migrate-test`(`api_test` / `auth_test`)。任意引数は `make -C app/migrator run ARGS="-target api -command status"` |
| 停止・片付け | ルート `make down`(`make clean` は named volume も削除) |

**マイグレーションも runtime レーン**(SPEC-016)。`make migrate` / `migrate-test` / `make -C app/migrator run` は compose の profile 付き `migrator` サービス = `app/migrator/Dockerfile` の runtime イメージで走る(distroless static / non-root / read-only rootfs / `cap_drop: ALL`、内部ネットワーク `dbnet` のみ、マイグレーション SQL はイメージに焼き込み)。**ローカル・CI・ECS の migration init コンテナが同一アーティファクト**になる。新しいマイグレーションファイルはイメージ再ビルドで反映されるため、各呼び出しは `--build` を付ける(`make` ターゲットが既にそうしている)。

toolchain レーンに残る唯一の Go バイナリ実行は `app/auth/cmd/keygen`(`make auth-signing-keys` / `rotate-signing-keys`)。常駐せず、リポジトリの `.secrets/` へ**書き込む**ことが目的なので、書き込み可能な bind mount を持つ toolbox 側が正しい置き場になる。常駐サーバーをこの例外に含めない。

### 最小権限・最小マウント(runtime コンテナ)

新規の runtime コンテナ(Dockerfile / compose サービス / ECS タスク定義)は最初からこれを満たす。適用状況の正は `compose.yml` / 各 `Dockerfile` / `app/iac/modules/service/ecs.tf` のコメント、経緯は ISSUE-056。

- **non-root**: Go は `gcr.io/distroless/static:nonroot` + `USER nonroot`、nginx は `nginxinc/nginx-unprivileged`(uid 101)で **非特権ポート 8080** を listen する(ホスト公開ポート 8080 / 8083 は不変)
- **`cap_drop: ALL`**(全サービス)。追加が要る場合のみ最小集合を理由付きで `cap_add`(現状の例外は postgres 公式イメージの初回 init に必要な chown / su-exec 系のみ)
- **`read_only: true` + 必要最小の tmpfs**(api / auth / web / auth-web)。ECS は `readonlyRootFilesystem: true` + `capabilities.drop: ["ALL"]`。postgres はローカル専用のため read_only を見送る(理由は `compose.yml` のコメントが正)
- **`security_opt: no-new-privileges:true`**(全サービス)
- **公開範囲**: ホストへの publish は `127.0.0.1` バインドのみ(`0.0.0.0` で公開しない)
- **ベースイメージは digest 固定**: `FROM <image>:<tag>@sha256:...`(タグは人間向けラベル、digest が同一性)。対象は api / auth / migrator の build+runtime ステージ、web / auth-web の build+runtime ステージ、compose の postgres(`versions.env` の `POSTGRES_VERSION` + `POSTGRES_IMAGE_DIGEST`。**必ず 2 つ同時に更新する** — digest が優先されるためタグだけ上げても切り替わらない)。すべて multi-arch の OCI *index* digest なので ECS の linux/arm64 も解決できる。更新手順は各 Dockerfile のコメント
- **egress は publish と両立しない**(2026-07-25 実測): egress を止める唯一の手段が `internal: true` ネットワークだが、この Docker(29.2.1)では internal ネットワークのみに接続したコンテナの publish ポートへホストから接続できない(busybox httpd の対照実験: 通常 bridge → 200 / internal → connection refused)。したがって publish する 5 サービスは default ネットワークに残し、**publish しない面(`tools-offline` = `network_mode: none`、`tools-db` / `migrator` = `dbnet`)だけ egress を落とす**。詳細と再測定手順は `compose.yml` の `networks:` コメント

### サプライチェーン(Shai-Hulud 型 npm ワーム)対策

npm エコシステムのコードがこのリポジトリで実行されるのは (1) toolchain コンテナの `make install`、(2) web / auth-web の **runtime イメージ build ステージ** の 2 箇所だけ。**両方に同じゲートを効かせる**(規約の正は `.claude/rules/web.md`、ゲート導入の経緯は ISSUE-020)。

- `bunfig.toml`(社内ミラー固定 + `minimumReleaseAge` = 21 日の隔離)を **`bun install` より前に COPY** する。build ステージが `bunfig.toml` を持たないと、そのビルドだけが既定の公開レジストリ・ゲート無しで解決される(= リポジトリ内で唯一の抜け道になる)
- build ステージは **`bun install --frozen-lockfile --ignore-scripts`**。lifecycle script(postinstall 等)は当該ワーム類の実行・流出経路そのもので、このビルドには不要(ネイティブ依存は optionalDependencies のプリビルドバイナリで供給される)
- **build context に資格情報と依存ツリーを入れない**。各 stack 直下の `.dockerignore` で `node_modules` / `dist` / `.env*` を除外する(ルートの `.dockerignore` は per-stack context には効かないので stack ごとに置く)。`node_modules` が入ると後段の `COPY . .` がゲート済みインストール結果をホストの木で上書きしてしまう。リポジトリルートを context に取るビルド(`app/migrator`)向けには、ルート `.dockerignore` が `.secrets/`(auth の RSA 署名鍵)と `.env*` を除外する
- ビルドにも実行にも `~/.npmrc` / クラウド資格情報 / docker.sock / SSH agent を渡さない。AWS 認証を要する操作(`push-images` / `deploy-web`)は host 側の `aws` CLI に限定する(SPEC-009 R7)
- runtime ステージに npm エコシステムを残さない。依存コードが動く面をビルド時に閉じ、常駐面(= 外部公開面)には持ち込まない

## ローカル実行(全スタック)

リポジトリルートの `Makefile` + `compose.yml` で 5 サービス(web / api / auth / auth-web と `postgres`。各 app は `app/*/Dockerfile` をビルド)をまとめて起動する: `make up`(フォアグラウンド)/ `make up-d`(バックグラウンド)/ `make down` / `make logs` / `make ps`。api・auth は compose 上では Postgres 経路(fail-closed)のため、`make up` / `up-d` は先に `make migrate`(`app/migrator` で api/auth の DB 作成 + マイグレーション。**SPEC-016 により compose の `migrator` サービス= `app/migrator/Dockerfile` の runtime イメージで実行**され、ECS の migration init コンテナと同一アーティファクトになる。ホストに Go は不要=前提は Docker のみ)を実行してからサービスを起動する。`make db-up`(postgres のみ)/ `make migrate` も個別に使える。起動後は web `http://localhost:8080` / api `http://localhost:8081` / auth `http://localhost:8082` / auth-web `http://localhost:8083` / postgres `127.0.0.1:5432`(ホストは `127.0.0.1` のみバインド)。web の nginx は `/api` / `/auth` を api / auth へ、auth-web の nginx は `/auth` を auth へ、それぞれ同一オリジンで proxy する(SPEC-015。OIDC issuer は `http://localhost:8080/auth`)。全ターゲットは `make help`。

- ここが **runtime レーンの正規経路**(前節「実行境界: runtime はコンテナのみ」)。単一サービスだけ起動したい場合は `make api-run` / `make auth-run`(= `make -C app/api run` / `make -C app/auth run`)で、compose の当該サービスを `--build` して起動する。**ホストで `go run` / `bun run dev` して代替しない**
- 各 stack のコマンドはルートから `make <stack>-<target>`(接頭辞 `api-` / `auth-` / `web-` / `auth-web-` / `iac-` / `migrator-`。例: `make api-check`、`make web-test TEST=<path>`、`make iac-plan ENV=dev`)でも呼べる(パターンルールのため `make help` の一覧には出ない)
- auth の署名鍵は compose 既定では ephemeral(再起動で失効)。永続化するには `make auth-signing-keys` で `.secrets/auth-signing-keys.json` を生成し、`compose.yml` のコメントアウトされた `SIGNING_KEYS_FILE` + mount を有効化する(ISSUE-036。ローテーションは `make rotate-auth-signing-keys`。`.secrets/` は絶対にコミットしない)

ルート `Makefile` には AWS デプロイ用の `push-images`(api/auth を ARM64 で ECR に build & push)/ `push-migrator-image`(migrator イメージ。apply 前に必要、ISSUE-017)/ `deploy-web`(web を build して S3 sync + CloudFront invalidation)もある(SPEC-004)。**これらは `terraform apply` 済み + AWS 認証情報を前提とする手動実行ターゲットで、agent は実行しない。**

## Git hooks (pre-commit)

コミット前に CI 相当の検証を走らせる git hooks を `.githooks/` に置いている。詳細は `.githooks/README.md` が正。

- **有効化(各 clone で 1 回)**: `make setup-hooks` → `core.hooksPath=.githooks`
- **手動実行**: `make hook-check`(ステージ済み変更を対象に pre-commit と同じ検証)
- **スキップ**: `git commit --no-verify` または `SKIP_PRE_COMMIT=1 git commit`

**何を走らせるか**: ステージ済みファイルのパスから `.github/workflows/cicd.yml` / `contract-drift.yml` / `sqlc-drift.yml` と同型の path filter で対象 stack を検出し、該当 stack の `make check`、必要なら contract drift / sqlc drift を実行する。api / auth stack がステージされた場合は check の前段で db-up + `make migrate-test` を行い、DB 依存テストを `REQUIRE_DB=1` で回す(SPEC-013)。migrator 自身の権限境界 integration ジョブ(`migrator-integration`)は含めない。

**実行環境(SPEC-009 準拠)**:

- **ホスト**: hook 本体が toolchain コンテナ内に再実行される(Docker 必須。warm = `tools` / offline 検査 = `tools-offline` / DB 依存テスト = `tools-db` の 3 フェーズ分割、SPEC-013 R6 / ISSUE-029)
- **devcontainer / `IN_TOOLBOX=1`**: そのセッション内で直接 `make check` 等を実行(Docker-in-Docker 不要)

docs-only や `.claude/` のみの変更など CI 対象外のステージはスキップする。

## CI(GitHub Actions)

CI/CD は 4 つの workflow に分かれる(担当は impl-ci)。上表の `make` 契約をコンテナ内で回すのは以下:

- **`cicd.yml`**(push / PR): メイン CI。先頭の `changes` job(`dorny/paths-filter`)で変更 stack を検出し、**変更のあった stack の job だけを起動**する(web / auth-web / api / auth / migrator / iac の各 `check` job と、DB 依存の `migrator-integration` job)。api / auth の `check` job は postgres service を起動し `make migrate-test`(`api_test` / `auth_test`)+ `REQUIRE_DB=1` で実 DB テストまで回す(SPEC-013 で旧 `api-integration` / `auth-integration` を `check` に統合)。「特定 stack の CI が走らない」ときはまずこの path-filter を疑う(workflow 自身の変更は全 job を再実行させる)
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
