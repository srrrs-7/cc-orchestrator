.DEFAULT_GOAL := help

# --- SPEC-009 Phase B: transparent toolbox delegation -----------------------
#
# Every target below (except `run`, `help`; see their own comments) is a
# thin wrapper around a `-native` counterpart. The command *names* this
# Makefile exposes are unchanged from before SPEC-009 (`make check`,
# `make test`, `make openapi`, ... -- see .claude/rules/api.md's command
# table, which this Makefile remains the single source of truth for);
# what changed is that the real recipe now runs inside the pinned
# toolbox container (.devcontainer/toolchain/Dockerfile via compose.tools.yml,
# docs/plans/SPEC-009-plan.md) instead of directly on the host, so a
# host with no go/golangci-lint/etc. installed can still run every
# target here.
#
# `IN_TOOLBOX` (baked into compose.tools.yml's `tools`/`tools-offline`/
# `tools-db` services' `environment:`) is the multi-wrap guard: GNU make's `ifdef`
# below is evaluated at parse time, so exactly one branch's targets
# exist in a given `make` invocation --
#   - IN_TOOLBOX unset (host):      only the public targets (`check`,
#     `test`, ...) exist; each `docker compose run`s into the toolbox
#     and invokes the matching `<target>-native` there.
#   - IN_TOOLBOX=1 (inside toolbox): only the `-native` targets exist;
#     there is no `check`/`test`/... target to accidentally re-delegate
#     from in here, so a `make check` run a second time inside an
#     already-toolboxed container fails fast (target not found) instead
#     of nesting another container.
REPO_ROOT := $(shell git rev-parse --show-toplevel)
DEVCONTAINER_DIR := $(REPO_ROOT)/.devcontainer
COMPOSE_TOOLS_FILE := $(DEVCONTAINER_DIR)/compose.tools.yml
include $(DEVCONTAINER_DIR)/versions.env

# Export every pinned version variable from versions.env as a real shell
# environment variable, so `docker compose`'s own `${VAR}` substitution
# (compose.tools.yml's build.args, compose.yml's postgres `image:`) can
# resolve them from the invoking process's environment. This used to be
# done via `docker compose --env-file $(REPO_ROOT)/versions.env` instead,
# but that flag is a *top-level* `docker compose` option (must appear
# before the subcommand) and some `docker compose` implementations (e.g.
# nerdctl/Rancher Desktop's compose plugin, confirmed on v5.3.1) reject it
# outright with "unknown flag: --env-file" even in that position -- so
# this Makefile no longer relies on it at all, only on plain env export,
# which every compose implementation honours identically.
export GO_VERSION BUN_VERSION GOLANGCI_LINT_VERSION TERRAFORM_VERSION TFLINT_VERSION TRIVY_VERSION SQLC_VERSION GOOSE_VERSION SWAG_VERSION GOIMPORTS_VERSION POSTGRES_VERSION

# TOOLBOX_UID/TOOLBOX_GID MUST be exported as real shell environment
# variables (not passed via `docker compose run -e`) for
# compose.tools.yml's `user: "${TOOLBOX_UID:-1000}:${TOOLBOX_GID:-1000}"`
# to resolve to the invoking host user: compose substitutes `${...}` in
# a YAML value from its own process environment (or --env-file) before
# the container is even created, while `-e`/`--env` only sets a
# variable *inside* the already-created container -- too late to affect
# `user:`. See compose.tools.yml's own header comment for the same
# note.
TOOLBOX_ENV := COMPOSE_PROJECT_NAME=cc-orchestrator TOOLBOX_UID="$$(id -u)" TOOLBOX_GID="$$(id -g)" \
	TOOLBOX_WORKSPACE="$(REPO_ROOT)" TOOLBOX_CONTEXT="$(DEVCONTAINER_DIR)/toolchain"

# docker compose (プラグイン) があればそれを使い、無ければ standalone
# docker-compose にフォールバックする(root Makefile / app/iac/Makefile /
# app/migrator/Makefile と同じ probe。一部のホストには compose plugin が
# 存在せず、`docker compose ...` がそのまま `unknown command`/`unknown
# shorthand flag` で落ちるため、実行時に解決する)。
COMPOSE_BIN := $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")

# Version substitution for compose.tools.yml's build args (and, via
# DB_COMPOSE below, compose.yml's postgres `image:`) comes from the
# `export`ed versions.env variables above, not `--env-file` -- see that
# export line's own comment for why.
COMPOSE    := $(COMPOSE_BIN) -f $(COMPOSE_TOOLS_FILE)
# `test`/`check`'s DB phase is the one exception that needs a reachable
# Postgres (SPEC-013: DB-backed tests are no longer gated behind a
# separate `//go:build integration` build; they are the default
# `test`): layering the sibling compose.yml (which owns the `postgres`
# service) on top lets `tools-db` join that project's network and
# resolve `postgres` by service name (SPEC-009 R6's revised network
# 切り分け表: `tools-db` can reach the test Postgres but not the
# internet; the fully offline `tools-offline` service cannot reach
# anything under network_mode:none).
DB_COMPOSE := $(COMPOSE_BIN) -f $(REPO_ROOT)/compose.yml -f $(COMPOSE_TOOLS_FILE)
WORKDIR    := --workdir /workspace/app/api

# tools-offline (network_mode:none): the execution service for every
# non-test step below -- fmt/fmt-check/lint/vet/build/openapi/sqlc/
# migrate-create all run fully offline (no deps/install phase exists in
# this Makefile that would need network).
OFFLINE  := $(TOOLBOX_ENV) $(COMPOSE) run --rm $(WORKDIR) tools-offline
# tools-db (postgres-reachable, no internet egress; SPEC-009 R6) +
# compose.yml layered in, for `test`'s DB phase only (see DB_COMPOSE
# above). Deliberately stops short of the service name/command:
# `docker compose run [OPTIONS] SERVICE [COMMAND...]` requires any `-e`
# flags to precede SERVICE (anything after it is parsed as the
# in-container command instead) -- `test` below appends its
# per-invocation `-e DB_*`/`-e REQUIRE_DB` flags, then `tools-db`, itself.
DB_ONLINE := $(TOOLBOX_ENV) $(DB_COMPOSE) run --rm $(WORKDIR)

# Tool CLIs previously invoked as `go run <pkg>@<version>` (a separate
# pinned version per stack Makefile). SPEC-009 Phase A found that `go
# run pkg@version` cannot resolve offline even with the exact
# module+version fully warmed in GOMODCACHE (it performs an
# unconditional module-proxy lookup every invocation) -- see
# compose.tools.yml's and .devcontainer/toolchain/Dockerfile's header comments
# for the verified detail. The toolbox image instead `go install`s each
# at its pinned version (versions.env's SQLC_VERSION / GOOSE_VERSION /
# SWAG_VERSION / GOIMPORTS_VERSION, `include`d above) onto
# GOBIN=/usr/local/go-tools/bin, already on PATH in every toolbox
# container regardless of volume/cache state -- so each is now invoked
# by its plain installed binary name. versions.env (not this file) is
# the single source of truth for which version that binary actually is.
GOIMPORTS := goimports
SWAG      := swag
GOOSE     := goose
SQLC      := sqlc

MIGRATIONS_DIR := infra/postgres/schema/migrations
SQLC_CONFIG := infra/postgres/sqlc.yaml

# Local-only, non-secret DB_* defaults (SPEC-005), matching
# compose.yml's `postgres` service and app/auth/Makefile's own copies.
# DB_HOST defaults to the compose *service* name `postgres` (not
# 127.0.0.1) because `test`'s DB phase now always runs inside the
# `tools-db` container joined to compose.yml's network (SPEC-009 R6)
# rather than directly on the host against a published port -- see
# `test` below, which forwards each of these into the container
# explicitly via `docker compose run -e` (docker isolates env by
# default; a plain host-shell `export DB_HOST=...`, this Makefile's
# pre-SPEC-009 contract, would not otherwise reach the containerized
# `go test` process). Every value is still overridable at invocation
# time, e.g. `make test DB_HOST=other-host`. DB_NAME defaults to the
# dedicated `api_test` database (SPEC-013 R3), never the `api` database
# `make up`/local development uses, so running tests never touches
# development data.
DB_HOST ?= postgres
DB_PORT ?= 5432
DB_NAME ?= api_test
DB_USER ?= app
DB_PASSWORD ?= app
DB_SSLMODE ?= disable

# REQUIRE_DB is deliberately NOT given a default here (no `?=`), and is
# forwarded to the DB-phase containers below as a bare `-e REQUIRE_DB`
# (no `=value`): pure environment passthrough (SPEC-013 §1.5) from the
# *caller's* shell environment as-is -- `docker compose run -e KEY` with
# no `=value` forwards the invoking process's own KEY, or leaves it
# unset if the caller never set it (same bare-forwarding form
# app/auth/Makefile's DB_TEST_ENV uses, so the two Makefiles' fail-closed
# forwarding cannot silently diverge). The regular check/CI/pre-commit
# paths (root Makefile, .github/workflows/cicd.yml, .githooks) set
# REQUIRE_DB=1 in their own environment before invoking this Makefile,
# which testsupport.OpenTestDB/RequireDBHost then turns into a hard
# t.Fatal (rather than a silent t.Skip) when DB_HOST is unset -- so a
# misconfigured DB_HOST cannot make DB-backed tests silently skip and
# appear green on those paths. An ad-hoc bare `make test`/`make check`
# (REQUIRE_DB unset) keeps the graceful-skip behavior for local,
# DB-less runs.

# `help`'s grep reads this file's raw text via $(MAKEFILE_LIST) --
# unlike a target lookup, that is not filtered by the ifdef/else split
# below, so both branches' doc comments are textually present
# regardless of IN_TOOLBOX. HELP_FILTER keeps `help`'s output matching
# whichever branch is actually active: the host-facing public targets
# outside the toolbox, or the `-native` ones inside it.
ifdef IN_TOOLBOX
HELP_FILTER := grep -E -- '-native:'
else
HELP_FILTER := grep -v -E -- '-native:'
endif

.PHONY: help
help: ## ターゲット一覧を表示する
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | $(HELP_FILTER) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

ifdef IN_TOOLBOX
# =============================================================================
# Inside the toolbox container (IN_TOOLBOX=1) -- the real recipes.
# =============================================================================

.PHONY: fmt-native
fmt-native: ## フォーマットを自動修正する (gofmt + goimports)
	gofmt -w .
	$(GOIMPORTS) -w .

.PHONY: fmt-check-native
fmt-check-native: ## フォーマット差分がないかチェックする
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then \
		echo "gofmt required:"; echo "$$files"; exit 1; \
	fi

.PHONY: lint-native
lint-native: ## golangci-lint を実行する
	golangci-lint run ./...

.PHONY: vet-native
vet-native: ## go vet を実行する
	go vet ./...

.PHONY: build-native
build-native: ## 全パッケージをビルドする
	go build ./...

.PHONY: test-native
test-native: ## テストを実行する (DB_* で指定した Postgres の api_test に対して実行。パッケージ間の truncate 隔離が競合しないよう -p 1。REQUIRE_DB=1 で DB_HOST 未設定時に fail する)
	go test -p 1 ./...

.PHONY: test-race-native
test-race-native: ## race detector 付きでテストを実行する (test-native 同様 DB 到達必須。-p 1 でパッケージ間の truncate 競合を防ぐ)
	go test -race -p 1 ./...

.PHONY: check-offline-native
check-offline-native: fmt-check-native lint-native vet-native build-native ## DB を要さないチェックのみ実行する (fmt-check + lint + vet + build)

.PHONY: check-native
check-native: check-offline-native test-native ## 全チェックを実行する (fmt-check + lint + vet + build + test)

.PHONY: openapi-native
openapi-native: ## OpenAPI 3.1 契約 (docs/openapi.yaml) を swag v2 注釈から生成する (check には含めない)
	$(SWAG) init -g cmd/api/main.go -o docs -ot yaml --v3.1
	mv docs/swagger.yaml docs/openapi.yaml

.PHONY: sqlc-native
sqlc-native: ## sqlc でクエリ (infra/postgres/schema/queries) からコード (infra/postgres/sqlcgen) を生成する (生成であり検査ではないため check には含めない)
	$(SQLC) generate -f $(SQLC_CONFIG)

.PHONY: migrate-create-native
migrate-create-native: ## 新しいマイグレーションファイルを作成する (例: make migrate-create name=add_foo_column)
	@if [ -z "$(name)" ]; then echo "usage: make migrate-create name=<slug>"; exit 1; fi
	$(GOOSE) -dir $(MIGRATIONS_DIR) create $(name) sql

else
# =============================================================================
# Host (IN_TOOLBOX unset) -- transparent delegation into the toolbox.
# Target names/semantics are unchanged (.claude/rules/api.md); only the
# execution environment moved into a container (SPEC-009 R4).
# =============================================================================

.PHONY: fmt
fmt: ## フォーマットを自動修正する (gofmt + goimports)
	$(OFFLINE) make fmt-native

.PHONY: fmt-check
fmt-check: ## フォーマット差分がないかチェックする
	$(OFFLINE) make fmt-check-native

.PHONY: lint
lint: ## golangci-lint を実行する
	$(OFFLINE) make lint-native

.PHONY: vet
vet: ## go vet を実行する
	$(OFFLINE) make vet-native

.PHONY: build
build: ## 全パッケージをビルドする
	$(OFFLINE) make build-native

.PHONY: test
test: ## テストを実行する (DB 到達必須。tools-db サービス + compose.yml の postgres [api_test] へ接続)
	$(DB_ONLINE) \
		-e DB_HOST=$(DB_HOST) -e DB_PORT=$(DB_PORT) -e DB_NAME=$(DB_NAME) \
		-e DB_USER=$(DB_USER) -e DB_PASSWORD=$(DB_PASSWORD) -e DB_SSLMODE=$(DB_SSLMODE) \
		-e REQUIRE_DB \
		tools-db make test-native

.PHONY: test-race
test-race: ## race detector 付きでテストを実行する (DB 到達必須。test 同様 tools-db 経由)
	$(DB_ONLINE) \
		-e DB_HOST=$(DB_HOST) -e DB_PORT=$(DB_PORT) -e DB_NAME=$(DB_NAME) \
		-e DB_USER=$(DB_USER) -e DB_PASSWORD=$(DB_PASSWORD) -e DB_SSLMODE=$(DB_SSLMODE) \
		-e REQUIRE_DB \
		tools-db make test-race-native

.PHONY: check
check: ## 全チェックを実行する (fmt-check + lint + vet + build [offline] + test [DB 到達, tools-db 経由])
	$(OFFLINE) make check-offline-native
	$(DB_ONLINE) \
		-e DB_HOST=$(DB_HOST) -e DB_PORT=$(DB_PORT) -e DB_NAME=$(DB_NAME) \
		-e DB_USER=$(DB_USER) -e DB_PASSWORD=$(DB_PASSWORD) -e DB_SSLMODE=$(DB_SSLMODE) \
		-e REQUIRE_DB \
		tools-db make test-native

.PHONY: openapi
openapi: ## OpenAPI 3.1 契約 (docs/openapi.yaml) を swag v2 注釈から生成する (check には含めない)
	$(OFFLINE) make openapi-native

.PHONY: sqlc
sqlc: ## sqlc でクエリ (infra/postgres/schema/queries) からコード (infra/postgres/sqlcgen) を生成する (生成であり検査ではないため check には含めない)
	$(OFFLINE) make sqlc-native

.PHONY: migrate-create
migrate-create: ## 新しいマイグレーションファイルを作成する (例: make migrate-create name=add_foo_column)
	$(OFFLINE) make migrate-create-native name=$(name)

endif

# `run` starts the API server itself (not a check/build/generate step),
# and is not part of SPEC-009's network 切り分け表 (`docker compose run`
# publishes no ports for tools/tools-offline, so wrapping this the same
# way would make the server unreachable from the host). Local full-stack
# serving already has a containerized path -- the root Makefile's
# `make up`/`up-d` (compose.yml's own `api` service, built from
# app/api/Dockerfile, with published ports) -- so this target is left
# as a host-side convenience for interactive development and is out of
# this Phase's scope, per SPEC-009 plan §手順 B1.
.PHONY: run
run: ## API サーバーを起動する (PORT 環境変数でポート変更可。ホスト go 実行。コンテナ経由のフルスタック起動はルート `make up` を使う)
	go run ./cmd/api
