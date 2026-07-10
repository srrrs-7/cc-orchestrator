.DEFAULT_GOAL := help

# --- SPEC-009 Phase B: transparent toolbox delegation -----------------------
#
# Every target below (except `run`, `help`; see their own comments) is a
# thin wrapper around a `-native` counterpart. The command *names* this
# Makefile exposes are unchanged from before SPEC-009 (`make check`,
# `make test`, `make sqlc`, ... -- see .claude/rules/auth.md's command
# table, which this Makefile remains the single source of truth for);
# what changed is that the real recipe now runs inside the pinned
# toolbox container (docker/toolchain/Dockerfile via compose.tools.yml,
# docs/plans/SPEC-009-plan.md) instead of directly on the host, so a
# host with no go/golangci-lint/etc. installed can still run every
# target here. This Makefile mirrors app/api/Makefile's wrapper shape
# 1:1 (SPEC-009 plan §手順 B1/B2 -- api and auth are meant to share one
# pattern), adjusted only for this stack's module path / DB name.
#
# `IN_TOOLBOX` (baked into compose.tools.yml's `tools`/`tools-offline`
# services' `environment:`) is the multi-wrap guard: GNU make's `ifdef`
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
include $(REPO_ROOT)/versions.env

# `docker compose`'s top-level `--env-file` flag is not portable across all
# `docker compose` implementations this Makefile is run against (e.g.
# nerdctl/Rancher Desktop's compose plugin v5.3.1 rejects it outright with
# `unknown flag: --env-file`), so this Makefile no longer passes it. Instead,
# every version variable this file itself `include`s from versions.env above
# is re-exported as a real make/shell environment variable; `docker compose`
# already resolves a YAML value's `${VAR}` substitution (compose.tools.yml's
# `build.args`, `user:`, etc.) from its own process environment with no
# `--env-file` needed, as long as the variable is actually exported into that
# process's environment (bare `include`d make variables are not -- hence the
# explicit `export` below).
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
TOOLBOX_ENV := TOOLBOX_UID="$$(id -u)" TOOLBOX_GID="$$(id -g)"

# docker compose (プラグイン) があればそれを使い、無ければ standalone
# docker-compose にフォールバックする(root Makefile / app/api/Makefile /
# app/iac/Makefile / app/migrator/Makefile と同じ probe。一部のホストには
# compose plugin が存在せず、`docker compose ...` がそのまま `unknown
# command`/`unknown shorthand flag` で落ちるため、実行時に解決する)。
COMPOSE_BIN := $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")

# Version substitution for compose.tools.yml's build args (and, via
# DB_COMPOSE below, compose.yml's postgres `image:`) comes from the
# `export`ed versions.env variables above, not `--env-file` -- see that
# export line's own comment for why.
COMPOSE    := $(COMPOSE_BIN) -f $(REPO_ROOT)/compose.tools.yml
# `test-integration` is the one exception that needs a reachable
# Postgres: layering the sibling compose.yml (which owns the `postgres`
# service) on top lets `tools` join that project's network and resolve
# `postgres` by service name (SPEC-009 plan network 切り分け表; the
# offline `tools-offline` service cannot reach it at all under
# network_mode:none).
DB_COMPOSE := $(COMPOSE_BIN) -f $(REPO_ROOT)/compose.yml -f $(REPO_ROOT)/compose.tools.yml
WORKDIR    := --workdir /workspace/app/auth

# tools-offline (network_mode:none): the default execution service for
# everything below except test-integration -- fmt/fmt-check/lint/vet/
# build/test/test-race/check/sqlc/migrate-create all run offline (no
# deps/install phase exists in this Makefile that would need network).
OFFLINE  := $(TOOLBOX_ENV) $(COMPOSE) run --rm $(WORKDIR) tools-offline
# tools (network enabled) + compose.yml layered in, for test-integration
# only (see DB_COMPOSE above).
DB_ONLINE := $(TOOLBOX_ENV) $(DB_COMPOSE) run --rm $(WORKDIR) tools

# Tool CLIs previously invoked as `go run <pkg>@<version>` (a separate
# pinned version per stack Makefile). SPEC-009 Phase A found that `go
# run pkg@version` cannot resolve offline even with the exact
# module+version fully warmed in GOMODCACHE (it performs an
# unconditional module-proxy lookup every invocation) -- see
# compose.tools.yml's and docker/toolchain/Dockerfile's header comments
# for the verified detail. The toolbox image instead `go install`s each
# at its pinned version (versions.env's SQLC_VERSION / GOOSE_VERSION /
# GOIMPORTS_VERSION, `include`d above) onto
# GOBIN=/usr/local/go-tools/bin, already on PATH in every toolbox
# container regardless of volume/cache state -- so each is now invoked
# by its plain installed binary name. versions.env (not this file) is
# the single source of truth for which version that binary actually is
# (keep GOOSE_VERSION there in sync with app/migrator/go.mod's
# github.com/pressly/goose/v3 require -- goose is a *library* dependency
# there, unlike the CLI usage here).
GOIMPORTS := goimports
GOOSE     := goose
SQLC      := sqlc

MIGRATIONS_DIR := db/migrations

# Local-only, non-secret DB_* defaults (SPEC-005), matching
# compose.yml's `postgres` service and app/api/Makefile's own copies.
# DB_HOST defaults to the compose *service* name `postgres` (not
# 127.0.0.1) because test-integration now always runs inside the
# `tools` container joined to compose.yml's network (SPEC-009) rather
# than directly on the host against a published port -- see
# test-integration below, which forwards each of these into the
# container explicitly via `docker compose run -e` (docker isolates
# env by default; a plain host-shell `export DB_HOST=...`, this
# Makefile's pre-SPEC-009 contract, would not otherwise reach the
# containerized `go test` process). Every value is still overridable at
# invocation time, e.g. `make test-integration DB_HOST=other-host`.
DB_HOST ?= postgres
DB_PORT ?= 5432
DB_NAME ?= auth
DB_USER ?= app
DB_PASSWORD ?= app
DB_SSLMODE ?= disable

.PHONY: help
help: ## ターゲット一覧を表示する
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

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
test-native: ## テストを実行する
	go test ./...

.PHONY: test-race-native
test-race-native: ## race detector 付きでテストを実行する
	go test -race ./...

.PHONY: check-native
check-native: fmt-check-native lint-native vet-native build-native test-native ## 全チェックを実行する (fmt-check + lint + vet + build + test)

.PHONY: sqlc-native
sqlc-native: ## db/queries から sqlcgen (infra/postgres/sqlcgen) を生成する (check には含めない。生成後は commit する)
	$(SQLC) generate

.PHONY: migrate-create-native
migrate-create-native: ## 新しい goose マイグレーションファイルを作成する (name=<slug> 必須)
	@if [ -z "$(name)" ]; then echo "usage: make migrate-create name=<slug>"; exit 1; fi
	$(GOOSE) -dir $(MIGRATIONS_DIR) create $(name) sql

.PHONY: test-integration-native
test-integration-native: ## infra/postgres の実 DB 統合テストを実行する (要: app/migrator で DB 作成 + up 適用済みの Postgres, DB_* 環境変数)
	go test -tags=integration ./infra/postgres/...

else
# =============================================================================
# Host (IN_TOOLBOX unset) -- transparent delegation into the toolbox.
# Target names/semantics are unchanged (.claude/rules/auth.md); only the
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
test: ## テストを実行する
	$(OFFLINE) make test-native

.PHONY: test-race
test-race: ## race detector 付きでテストを実行する
	$(OFFLINE) make test-race-native

.PHONY: check
check: ## 全チェックを実行する (fmt-check + lint + vet + build + test)
	$(OFFLINE) make check-native

.PHONY: sqlc
sqlc: ## db/queries から sqlcgen (infra/postgres/sqlcgen) を生成する (check には含めない。生成後は commit する)
	$(OFFLINE) make sqlc-native

.PHONY: migrate-create
migrate-create: ## 新しい goose マイグレーションファイルを作成する (name=<slug> 必須)
	$(OFFLINE) make migrate-create-native name=$(name)

.PHONY: test-integration
test-integration: ## infra/postgres の実 DB 統合テストを実行する (offline 不可。tools サービス + compose.yml の postgres へ接続。事前に app/migrator で DB 作成 + up 適用が必要)
	$(DB_ONLINE) \
		-e DB_HOST=$(DB_HOST) -e DB_PORT=$(DB_PORT) -e DB_NAME=$(DB_NAME) \
		-e DB_USER=$(DB_USER) -e DB_PASSWORD=$(DB_PASSWORD) -e DB_SSLMODE=$(DB_SSLMODE) \
		make test-integration-native

endif

# `run` starts the authz server itself (not a check/build/generate step),
# and is not part of SPEC-009's network 切り分け表 (`docker compose run`
# publishes no ports for tools/tools-offline, so wrapping this the same
# way would make the server unreachable from the host). Local full-stack
# serving already has a containerized path -- the root Makefile's
# `make up`/`up-d` (compose.yml's own `auth` service, built from
# app/auth/Dockerfile, with published ports) -- so this target is left
# as a host-side convenience for interactive development and is out of
# this Phase's scope, per SPEC-009 plan §手順 B2.
.PHONY: run
run: ## 認可サーバーを起動する (PORT / ISSUER 環境変数で変更可。ホスト go 実行。コンテナ経由のフルスタック起動はルート `make up` を使う)
	go run ./cmd/authz
