.DEFAULT_GOAL := help

# docker compose(プラグイン)があればそれを使い、無ければ standalone docker-compose にフォールバックする
COMPOSE := $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")

# `help`'s grep below needs `-h` (suppress filename prefix): further down,
# `include $(DEVCONTAINER_DIR)/versions.env` adds a second entry to $(MAKEFILE_LIST),
# so a plain `grep` would otherwise prefix every matched line with its
# source filename ("Makefile:build: ## ...") once more than one file is in
# that list, which breaks this awk's ":.*?## "-based field split (the
# target-name column would print the filename instead of the target).
# ---------------------------------------------------------------------------
# Git hooks — pre-commit runs CI-equivalent checks for staged changes only
# (see .githooks/README.md). SPEC-013 (テストの実 DB 一本化): api/auth `make
# check` is now test-DB-backed by default (formerly the separate
# `*-integration` jobs' concern, folded into `check` itself), so a staged
# api/auth change now provisions Postgres + the `api_test`/`auth_test`
# databases (`db-up` + `migrate-test`, this same file's own targets) before
# running that stack's `check` -- this is no longer "excluded" the way it
# used to be. Only `migrator`'s own separate Postgres-permission-boundary
# suite (CI's `migrator-integration` job, ISSUE-016 R-c) stays out of scope
# here -- that is a distinct, slower concern unrelated to api/auth's own
# persistence-layer tests and is not part of this hook. On the host,
# hook-check re-enters the toolchain container; inside devcontainer
# (IN_TOOLBOX=1) it runs directly.
# ---------------------------------------------------------------------------

.PHONY: setup-hooks
setup-hooks: ## git hooks を有効化する (core.hooksPath=.githooks)
	git config core.hooksPath .githooks
	@echo "Git hooks enabled (core.hooksPath=.githooks). Run 'make hook-check' to test manually."

.PHONY: hook-check
hook-check: ## pre-commit と同じ CI 検証を手動実行する (ステージ済み変更対象)
	@bash ./.githooks/lib/run-checks.sh

.PHONY: help
help: ## ターゲット一覧を表示する (起動後: web http://localhost:8080 / api http://localhost:8081 / auth http://localhost:8082)
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

.PHONY: build
build: ## 全サービスのイメージをビルドする
	$(COMPOSE) build

.PHONY: up
up: migrate ## postgres を起動・マイグレーション適用した上でビルドしてフォアグラウンドで起動する
	$(COMPOSE) up --build

.PHONY: up-d
up-d: migrate ## postgres を起動・マイグレーション適用した上でビルドしてデタッチ(バックグラウンド)で起動する
	$(COMPOSE) up -d --build

.PHONY: down
down: ## 全サービスを停止・コンテナを削除する
	$(COMPOSE) down

.PHONY: logs
logs: ## 全サービスのログを追従する
	$(COMPOSE) logs -f

.PHONY: ps
ps: ## 稼働状況を表示する
	$(COMPOSE) ps

.PHONY: restart
restart: ## 全サービスを再起動する
	$(COMPOSE) restart

.PHONY: clean
clean: ## 停止・コンテナ・volume を削除する
	$(COMPOSE) down -v

# ---------------------------------------------------------------------------
# ローカル Postgres(SPEC-005。2026-07-09 リファクタ: 別データベース + app/migrator。
# 2026-07-10 SPEC-009 Phase B: `migrate` の実行環境をコンテナ化)
#
# api/auth はどちらも起動時に DB_HOST の有無で永続化先を選ぶ fail-closed な
# 配線になっている(app/{api,auth}/infra/postgres/db.go の SelectMode)。
# compose.yml は api/auth に DB_HOST=postgres 他の DB_* を注入済みのため、
# `up`/`up-d` は必ず Postgres 経路を使う(memory 経路には戻らない)。
# マイグレーション未適用のテーブル欠如でクラッシュループしないよう、
# `up`/`up-d` は `migrate`(→ `db-up`)を前提ターゲットにしている。
#
# api・auth は同一 Postgres インスタンス上の別データベース("api"/"auth")に
# 分離されている(旧: 単一 database + search_path 別スキーマ)。`migrate` は
# 共通の `app/migrator`(`app/migrator/cmd/migrator/main.go`)を `-target` 違いで
# 2 回実行し、各データベースを(未存在なら)作成した上で当該スタックの schema/migrations を
# 適用する。DB_NAME は migrator の既定(-target と同名: api/auth)に任せ、
# 接続先(host/port/user/password/sslmode)だけをローカル compose の postgres
# に合わせて明示する(app/api・app/auth Makefile の DB_* 既定と同じ値)。
# ---------------------------------------------------------------------------

# ---------------------------------------------------------------------------
# Containerized toolchain wiring (SPEC-009 Phase B; foundation under
# .devcontainer/: toolchain/, compose.tools.yml, versions.env). `migrate`
# below and `deploy-web`'s build step (further down) previously
# shelled out to a *host* language runtime (`go`/`bun`) -- both are
# wrapped here to run inside the pinned toolchain image instead. `build`/
# `up`/`up-d`/`down`/`logs`/`ps`/`restart`/`clean`, and `push-images`'s
# `docker buildx` image build, are plain docker/compose *operations*, not
# language-toolchain invocations, so they need no such wrapping (SPEC-009
# plan §手順 フェーズ B). `db-up` is the one partial exception among these
# "plain operation" targets: it still only ever starts the `postgres`
# service (no toolchain container involved), but -- see COMPOSE_DB_FILES
# just below -- its *compose file combination* must be aligned with
# `migrate`'s, for a network-consistency reason unrelated to toolchain
# wrapping itself.
# Pinned tool/runtime versions (SPEC-009's single source of truth), needed
# here so `docker compose`'s own `${VAR}` substitution (compose.tools.yml's
# build.args, compose.yml's postgres `image:`) can resolve them. This used
# to be done via `docker compose --env-file $(CURDIR)/.devcontainer/versions.env` instead,
# but that flag is a *top-level* `docker compose` option and some `docker
# compose` implementations (e.g. nerdctl/Rancher Desktop's compose plugin,
# confirmed on v5.3.1) reject it outright with "unknown flag: --env-file" --
# so this Makefile no longer relies on it at all, only on plain env export,
# which every compose implementation honours identically (same fix as
# app/api/Makefile's own copy of this same comment).
DEVCONTAINER_DIR := $(CURDIR)/.devcontainer
COMPOSE_TOOLS_FILE := $(DEVCONTAINER_DIR)/compose.tools.yml
include $(DEVCONTAINER_DIR)/versions.env
export GO_VERSION BUN_VERSION GOLANGCI_LINT_VERSION TERRAFORM_VERSION TFLINT_VERSION TRIVY_VERSION SQLC_VERSION GOOSE_VERSION SWAG_VERSION GOIMPORTS_VERSION POSTGRES_VERSION

TOOLBOX_UID := $(shell id -u)
TOOLBOX_GID := $(shell id -g)
# The toolchain compose file now lives under .devcontainer/ (so the VS
# Code Dev Containers extension can build it directly). Its build context
# and /workspace bind-mount default to that directory's own layout
# (./toolchain, ..) for the editor; every CLI/CI caller instead passes
# *absolute*, repo-root-anchored TOOLBOX_CONTEXT/TOOLBOX_WORKSPACE so
# those paths are correct no matter which project directory Compose
# derives from the `-f` list (see .devcontainer/compose.tools.yml's own
# header). COMPOSE_PROJECT_NAME is pinned to the same value all root-
# Makefile compose targets already default to (this repo dir's basename),
# so a tools-only invocation -- whose first `-f` is now under
# .devcontainer/ -- does not silently switch the project (and thus the
# named cache volumes / default network) to "devcontainer".
TOOLBOX_WORKSPACE := $(CURDIR)
TOOLBOX_CONTEXT   := $(DEVCONTAINER_DIR)/toolchain
# TOOLBOX_UID/TOOLBOX_GID (and TOOLBOX_CONTEXT/TOOLBOX_WORKSPACE) must be
# real *shell* environment variables -- not `-e` flags on `docker compose
# run` -- for compose.tools.yml's `${...}` interpolation (`user:`,
# `build.context`, the workspace volume source) to resolve at
# container-creation time (see that file's own header comment on why
# `-e`, which only sets a variable *inside* the already-created
# container, is too late). Prefixing each invocation below achieves this
# without a persistent `export`.
TOOLBOX_ENV := COMPOSE_PROJECT_NAME=cc-orchestrator TOOLBOX_UID=$(TOOLBOX_UID) TOOLBOX_GID=$(TOOLBOX_GID) TOOLBOX_WORKSPACE=$(TOOLBOX_WORKSPACE) TOOLBOX_CONTEXT=$(TOOLBOX_CONTEXT)

# Compose file combination shared by `db-up` below and `migrate`'s
# DB_TOOLS_RUN further down: both MUST resolve to the *exact same*
# docker-compose project config (same exported version env, same
# `-f ... -f ...` file list), so that the CLI computes an identical
# config hash for the implicit default network on every invocation.
#
# Observed empirically during Phase B implementation: `db-up` originally
# ran bare `$(COMPOSE) up -d --wait postgres` (compose.yml alone), while
# `migrate` (via DB_TOOLS_RUN) ran `-f compose.yml -f .devcontainer/compose.tools.yml`.
# Even though neither file declares an explicit top-level `networks:`
# (both rely on the same implicit `<project>_default` network, and the
# project name is identical either way -- derived from this directory,
# no COMPOSE_PROJECT_NAME override), docker compose detected the config
# mismatch between these two *separate* invocations and recreated the
# default network for the second one: the already-running `postgres`
# container (attached under the first invocation's network object)
# stayed attached to the now-orphaned old network, while `migrate`'s new
# `tools` container joined the freshly recreated one -- same network
# *name*, different object underneath, so the two could no longer reach
# each other by service name (`DB_HOST=postgres` resolution failed).
#
# Aligning `db-up` to this exact same file combination removes the
# mismatch: both invocations now hash identically, so no recreation
# happens between them and `migrate` can resolve `postgres` by name.
#
# `up`/`up-d` do NOT need this same treatment and are left on plain
# compose.yml (see those targets' own recipes above): unlike the
# `db-up` → (separate) `migrate` pair, `up`/`up-d`'s `$(COMPOSE) up
# --build` brings up postgres/api/auth/web together in one single
# reconciled invocation -- if docker compose needs to recreate the
# network at that point, it does so as part of that same invocation and
# reconnects all four containers to it together, so no split-network
# state can persist by the time it finishes. The bug above was
# specifically about two *separate* invocations (`db-up` alone, then
# `migrate` alone) disagreeing with each other; `up`/`up-d` never split
# postgres's startup from api/auth/web's this way.
COMPOSE_DB_FILES := -f compose.yml -f $(COMPOSE_TOOLS_FILE)

.PHONY: db-up
db-up: ## postgres のみを起動し healthy になるまで待つ(migrate と同じ compose ファイル構成で network 整合を保つ)
	$(TOOLBOX_ENV) $(COMPOSE) $(COMPOSE_DB_FILES) up -d --wait postgres

# `tools` (network enabled), merged with this same file's own
# compose.yml so the container can reach its `postgres` service by name.
# Used only by `migrate` below -- the one root-Makefile phase that needs
# a real Postgres (SPEC-009 plan's network-要否 table: "root make
# migrate(app/migrator 実行)" row is a `tools`(+compose.yml) exception,
# not `tools-offline`). Shares COMPOSE_DB_FILES with `db-up` above so
# both resolve to the identical compose project config (see that
# variable's own comment for why this matters).
DB_TOOLS_RUN := $(TOOLBOX_ENV) $(COMPOSE) $(COMPOSE_DB_FILES) run --rm

# `tools-offline` (network_mode: none). Reserved for root Makefile targets
# that need a one-off offline exec without going through a stack Makefile.
# (Currently unused — stack Makefiles own their own OFFLINE delegation.)

MIGRATOR_DB_ENV_FLAGS := -e DB_HOST=postgres -e DB_PORT=5432 -e DB_USER=app -e DB_PASSWORD=app -e DB_SSLMODE=disable

# `go run ./cmd/migrator` now runs *inside* the `tools` container above
# (DB_TOOLS_RUN), with `-w /workspace/app/migrator` as its working
# directory -- app/migrator's own go.mod-rooted module, reached this way
# instead of the previous host-side `go -C app/migrator run ...` (this
# repo root has no go.mod of its own, so `-C`/`-w` into app/migrator's
# module context is still required; only *where* that `go run` executes
# has changed). `-migrations-dir` is therefore rooted at /workspace (the
# container's bind-mounted view of this repo, per compose.tools.yml),
# not $(CURDIR) (the host's view) -- app/api's and app/auth's
# schema/migrations directories are bind-mounted at exactly
# /workspace/app/{api,auth}/infra/postgres/schema/migrations. DB_HOST=postgres (a compose
# service *name*, not 127.0.0.1) because this container joins the
# postgres service's own compose network via the `-f compose.yml -f
# compose.tools.yml` file overlay above, rather than reaching it through
# compose.yml's host-published 127.0.0.1:5432 port the previous
# host-side `go run` used.
.PHONY: migrate
migrate: db-up ## api/auth のデータベースをローカル compose の postgres に作成・マイグレーション適用する (app/migrator 経由。db-up を前提として実行。toolchain コンテナ内で go を実行し、postgres へは compose ネットワーク越しに到達する)
	$(DB_TOOLS_RUN) -w /workspace/app/migrator $(MIGRATOR_DB_ENV_FLAGS) tools go run ./cmd/migrator -target api -migrations-dir /workspace/app/api/infra/postgres/schema/migrations
	$(DB_TOOLS_RUN) -w /workspace/app/migrator $(MIGRATOR_DB_ENV_FLAGS) tools go run ./cmd/migrator -target auth -migrations-dir /workspace/app/auth/infra/postgres/schema/migrations

# ---------------------------------------------------------------------------
# SPEC-013 (テストの実 DB 一本化): dedicated *test* databases, kept fully
# separate from the dev `api`/`auth` databases `migrate` above provisions
# (plan §1.3 "test DB 導線" -- app/migrator itself needs no code change,
# since its own `DB_NAME` env var already defaults to `-target`'s name but
# is happily overridden). `api_test`/`auth_test` (underscore, not hyphen)
# because app/migrator's own identifier validation
# (`domain/migration.ParseDatabaseName`, `^[a-z_][a-z0-9_]*$`) rejects a
# hyphen outright (SPEC-013 §4 "test DB 命名").
#
# Mirrors `migrate` above exactly (same DB_TOOLS_RUN invocation shape, same
# `-migrations-dir`s, same lack of any APP_DB_USER/APP_DB_PASSWORD role-
# provisioning flags -- test databases have no least-privilege runtime role
# to provision, unlike ISSUE-016's api_app/auth_app convention for the real
# dev/prod databases), with only MIGRATOR_DB_ENV_FLAGS's DB_NAME overridden
# per invocation (`-e DB_NAME=...` placed *after* the shared flags so it
# wins -- `docker compose run -e` flags are applied left-to-right, last one
# for a given name set takes effect).
MIGRATOR_TEST_DB_ENV_FLAGS_API  := $(MIGRATOR_DB_ENV_FLAGS) -e DB_NAME=api_test
MIGRATOR_TEST_DB_ENV_FLAGS_AUTH := $(MIGRATOR_DB_ENV_FLAGS) -e DB_NAME=auth_test

.PHONY: migrate-test
migrate-test: db-up ## api_test/auth_test テスト用データベースをローカル compose の postgres に作成・マイグレーション適用する (app/migrator 経由。db-up を前提。開発用 api/auth データベースとは別。role provisioning env は渡さない)
	$(DB_TOOLS_RUN) -w /workspace/app/migrator $(MIGRATOR_TEST_DB_ENV_FLAGS_API) tools go run ./cmd/migrator -target api -migrations-dir /workspace/app/api/infra/postgres/schema/migrations
	$(DB_TOOLS_RUN) -w /workspace/app/migrator $(MIGRATOR_TEST_DB_ENV_FLAGS_AUTH) tools go run ./cmd/migrator -target auth -migrations-dir /workspace/app/auth/infra/postgres/schema/migrations

# Convenience target bundling the full local "run DB-backed tests" flow:
# start Postgres + provision both test databases (`migrate-test`, which
# itself depends on `db-up`), then run each stack's own `test` target
# (app/api|app/auth Makefile -- SPEC-013 T4a/T4b make these connect to
# `api_test`/`auth_test` by default once that target's testsupport default
# is updated) against them. REQUIRE_DB=1 is exported as a real *process*
# environment variable (not a `make`-only command-line override) for both
# sub-`make` invocations below, following this repo's existing `?=`-default
# convention (e.g. DB_HOST ?= ... in app/api|app/auth/Makefile): a stack
# Makefile that declares `REQUIRE_DB ?=` and forwards it into its own
# container invocation (`-e REQUIRE_DB=$(REQUIRE_DB)`, mirroring DB_HOST's
# own forwarding) picks this up automatically, with no further plumbing
# needed here. This is the SPEC-013 plan §1.5 admin ruling's "正規経路":
# DB tests must fail loudly (t.Fatal) rather than silently t.Skip if the
# test DB connection is ever unavailable through this path.
.PHONY: test-db
test-db: migrate-test ## test DB を用意して api/auth の DB 到達テストを実行する (migrate-test 前提。REQUIRE_DB=1 を注入し黙った skip を防ぐ)
	REQUIRE_DB=1 $(MAKE) -C app/api test
	REQUIRE_DB=1 $(MAKE) -C app/auth test

# ---------------------------------------------------------------------------
# ISSUE-036: 署名鍵リング生成(app/auth RSA key persistence)
# ---------------------------------------------------------------------------

.PHONY: auth-signing-keys
auth-signing-keys: ## auth の RSA 署名鍵リングを生成する (.secrets/auth-signing-keys.json。gitignore 済み。秘密鍵を含むため絶対にコミットしない。事前に keygen ツールが app/auth/cmd/keygen に必要)
	$(MAKE) -C app/auth auth-signing-keys

.PHONY: rotate-auth-signing-keys
rotate-auth-signing-keys: ## auth の RSA 署名鍵をローテーションする (旧 active 鍵を verify-only に降格し新しい active 鍵を追加)
	$(MAKE) -C app/auth rotate-signing-keys

# ---------------------------------------------------------------------------
# AWS デプロイ(build-push)ツーリング(SPEC-004。2026-07-10 SPEC-009 Phase B:
# deploy-web の build ステップをコンテナ化)
#
# 前提: app/iac/envs/dev が `terraform apply` 済みで、実行環境に AWS 認証情報
# (`aws configure` 等)が設定されていること。ECR リポジトリ URL・S3 バケット名・
# CloudFront distribution ID は `terraform output`(app/iac/envs/dev)から取得
# する — 参照する output 名の契約は app/iac/envs/dev/outputs.tf が正。
#
# apply 済み + AWS 認証情報が前提のため、agent はこれらのターゲットを実行
# しない(手動実行前提)。ARM64 を明示指定するのは、amd64 で誤ビルドすると
# ECS の ARM64 タスク定義でコンテナが起動できない不具合(ISSUE-014)を防ぐため。
#
# SPEC-009 R7 (build=コンテナ / aws=host の分割): `push-images` はもともと
# `docker buildx build`(app/*/Dockerfile。app のビルド用イメージで、この
# SPEC のスコープ外)と `aws ecr`/`docker login` のみで、host の言語
# ランタイム(go/bun)を直接叩く箇所が無いため変更不要。`deploy-web` の
# build ステップは `app/web/Makefile` 経由で toolchain コンテナ内のみ実行する。
# どちらのターゲットも
# `aws s3`/`aws cloudfront`/`aws ecr`/`docker login`/`docker buildx` は
# 引き続き host 実行のまま(AWS 認証情報を toolchain コンテナに渡さない)。
# ---------------------------------------------------------------------------

AWS_REGION ?= ap-northeast-1
IMAGE_TAG ?= latest
TF_ENV_DIR := app/iac/envs/dev

.PHONY: push-images
push-images: ## api/auth イメージを ARM64 で ECR に build & push する(apply 済み + AWS 認証情報が前提。agent は実行しない・手動実行前提)
	@api_repo="$$(terraform -chdir=$(TF_ENV_DIR) output -raw api_ecr_repository_url)" && \
	auth_repo="$$(terraform -chdir=$(TF_ENV_DIR) output -raw auth_ecr_repository_url)" && \
	registry="$$(echo "$$api_repo" | cut -d/ -f1)" && \
	aws ecr get-login-password --region $(AWS_REGION) | docker login --username AWS --password-stdin "$$registry" && \
	docker buildx build --platform linux/arm64 --push -t "$$api_repo:$(IMAGE_TAG)" app/api && \
	docker buildx build --platform linux/arm64 --push -t "$$auth_repo:$(IMAGE_TAG)" app/auth

# ISSUE-017: 共有 app/migrator イメージを ARM64 で migrator 専用 ECR に build & push する。
# ビルドコンテキストはリポジトリルート(.) — app/migrator/Dockerfile が
# app/api/infra/postgres/schema/migrations と app/auth/infra/postgres/schema/migrations の
# 両方を COPY するためコンテキストが app/migrator ではなくルートである必要がある。
# タグは :latest — iac が参照する migration_image の既定値(app/iac/envs/dev/main.tf)に合わせる。
# 1 イメージで api/auth 双方の migrate init コンテナをカバーするため、push は 1 回のみ。
# apply 前にこのターゲットを実行すること(app/iac/envs/dev/README.md「apply 前の前提条件」)。
.PHONY: push-migrator-image
push-migrator-image: ## app/migrator イメージを ARM64 で ECR に build & push する(apply 済み + AWS 認証情報が前提。agent は実行しない・手動実行前提。ISSUE-017)
	@migrator_repo="$$(terraform -chdir=$(TF_ENV_DIR) output -raw migrator_ecr_repository_url)" && \
	registry="$$(echo "$$migrator_repo" | cut -d/ -f1)" && \
	aws ecr get-login-password --region $(AWS_REGION) | docker login --username AWS --password-stdin "$$registry" && \
	docker buildx build --platform linux/arm64 --push -f app/migrator/Dockerfile -t "$$migrator_repo:latest" .

.PHONY: deploy-web
deploy-web: ## web を build(app/web/Makefile 経由・toolchain コンテナ内)して S3 sync + CloudFront invalidation する(apply 済み + AWS 認証情報が前提。agent は実行しない・手動実行前提)
	$(MAKE) -C app/web build
	aws s3 sync app/web/dist "s3://$$(terraform -chdir=$(TF_ENV_DIR) output -raw web_bucket_name)" --delete
	aws cloudfront create-invalidation --distribution-id "$$(terraform -chdir=$(TF_ENV_DIR) output -raw cloudfront_distribution_id)" --paths '/*'

# ---------------------------------------------------------------------------
# 各スタックへのパススルー(root から各ディレクトリのコマンドを実行するための
# 薄い委譲)。実体と toolchain コンテナ委譲は各スタックの Makefile(app/api・
# app/auth・app/iac・app/migrator・app/web)側にあり、ここは
# `$(MAKE) -C ...` を呼ぶだけ。コマンド名の正は
# .claude/rules/<stack>.md と各 Makefile のままで、これらは利便性のための別名。
#
# 名前衝突を避けるためスタック接頭辞(api- / auth- / iac- / migrator- / web-)を
# 付ける(root には build/up/down/migrate 等の同名ターゲットが既にあるため)。
# コマンドライン変数(name=... / ENV=...)は sub-make に自動で伝播する。
#
# 例:
#   make api-check / make auth-test / make iac-plan ENV=prod
#   make api-migrate-create name=add_foo_column
#   make web-typecheck / make web-test
#
# 注: これらはパターンルールのため `make help` の一覧には出ない
# (help の grep は `^[a-zA-Z_-]+:` のみに一致する)。使い方はこのコメントと、
# 委譲先の `make api-help` 等(各スタックの help)を参照。web の単一テストなど
# パス引数を渡したい場合は `make web-test TEST=<path>` または
# `make -C app/web test TEST=<path>` を使う。
# ---------------------------------------------------------------------------
api-%:
	$(MAKE) -C app/api $*

auth-%:
	$(MAKE) -C app/auth $*

iac-%:
	$(MAKE) -C app/iac $*

migrator-%:
	$(MAKE) -C app/migrator $*

web-%:
	$(MAKE) -C app/web $*

auth-web-%:
	$(MAKE) -C app/auth-web $*
