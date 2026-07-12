.DEFAULT_GOAL := help

# ---------------------------------------------------------------------------
# SPEC-009 Phase B: containerized toolchain wrapper.
#
# Every public target below (fmt/fmt-check/lint/vet/build/test/test-race/
# check/run/test-integration) keeps its pre-SPEC-009 name and behaviour
# (R4: the command contract does not change) but, when invoked on the
# host, transparently re-runs itself inside the pinned toolchain image
# (.devcontainer/toolchain, compose.tools.yml) instead of shelling out to a
# host-installed go/golangci-lint. The actual recipes live in the
# `-native` targets guarded by `ifdef IN_TOOLBOX` below; that variable is
# already exported as a container env var by compose.tools.yml's
# x-tools-env anchor for both the `tools` and `tools-offline` services,
# so a `-native` target never re-wraps itself into a second, nested
# `docker compose run` when it executes inside the container that set it.
#
# See docs/plans/SPEC-009-plan.md "Makefile 透過ラッパーの形" and
# .claude/rules/db.md for the command/network contract this follows.
# ---------------------------------------------------------------------------

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

MODULE_DIR := app/migrator

# docker compose (プラグイン) があればそれを使い、無ければ standalone
# docker-compose にフォールバックする (root Makefile の $(COMPOSE) と同じ
# probe。.devcontainer/toolchain/README.md もこのフォールバックを前提にしている)。
COMPOSE_BIN := $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")

# TOOLBOX_UID/TOOLBOX_GID must be plain *shell* environment variables
# (not `-e`/`docker compose run -e`), because compose resolves
# compose.tools.yml's `user: "${TOOLBOX_UID:-1000}:${TOOLBOX_GID:-1000}"`
# from its own process environment before the container is even
# created -- a `-e` flag only sets a variable inside the
# already-created container, too late to affect `user:` (see that
# file's own header comment).
# Version substitution for compose.tools.yml's build args (and, via
# DB_COMPOSE_TOOLS below, compose.yml's postgres `image:`) comes from the
# `export`ed versions.env variables above, not `--env-file` -- see that
# export line's own comment for why.
TOOLBOX_ENV_PREFIX := COMPOSE_PROJECT_NAME=cc-orchestrator \
	TOOLBOX_UID=$(shell id -u) TOOLBOX_GID=$(shell id -g) \
	TOOLBOX_WORKSPACE=$(REPO_ROOT) TOOLBOX_CONTEXT=$(DEVCONTAINER_DIR)/toolchain
COMPOSE_TOOLS := $(TOOLBOX_ENV_PREFIX) \
	$(COMPOSE_BIN) -f $(COMPOSE_TOOLS_FILE)

# network_mode:none -- fmt/fmt-check/lint/vet/build/test/test-race/check
# never need the network themselves (this module's own go.sum deps are
# pre-warmed into the gomodcache/gobuild named volumes by a one-time
# `go mod download` via the network-enabled `tools` service first --
# see .devcontainer/toolchain/README.md's "検証" section -- and GOPROXY=off
# inside tools-offline makes any *unexpected* fetch fail loudly instead
# of silently reaching out).
OFFLINE := $(COMPOSE_TOOLS) run --rm tools-offline make -C $(MODULE_DIR)

# `tools` (network enabled, no DB reachability needed) -- reserved for
# phases that must reach the module proxy but not Postgres. `go mod
# tidy` needs to resolve/verify module metadata (and fetch anything not
# already in the shared gomodcache volume, e.g. after a manual go.mod
# edit or a merge conflict), which tools-offline's GOPROXY=off would
# make fail. Same lane/rationale as app/web/Makefile's `install` and
# app/api/Makefile's own `tidy` (RETRO-004 / RETROSUM-001 提案 3):
# unlike ONLINE_DB below, this does NOT layer the sibling compose.yml on
# top, since `go mod tidy` never needs to reach the `postgres` service.
ONLINE := $(COMPOSE_TOOLS) run --rm tools make -C $(MODULE_DIR)

# `tools` (network enabled) layered on top of the sibling root
# compose.yml, so the `postgres` service resolves by its compose
# service name on their shared network -- this module's own `run`
# target (the migrator binary itself, which only ever does anything
# useful against a real Postgres) and `test-integration` (ISSUE-016's
# privilege-boundary tests) both need real DB reachability, so neither
# can run under network_mode:none. SPEC-009 plan's network cutover
# table lists both "root make migrate(app/migrator 実行)" and "make
# test-integration" as `tools`(+compose.yml) exceptions, not
# `tools-offline`.
#
# DB_HOST=postgres (not 127.0.0.1) is hardcoded here deliberately: from
# inside a container joined to compose.yml's network, the postgres
# service is reachable by its compose service name, not via the
# 127.0.0.1-published host port (that mapping only helps callers
# running directly on the host, which is exactly the DB_HOST ?= 127.0.0.1
# default the test-integration-native/run-native recipes below still
# fall back to when invoked directly, e.g. inside an already-toolboxed
# shell without this DB overlay). The rest of these values simply match
# the sibling compose.yml's own postgres service credentials
# (POSTGRES_USER=app/POSTGRES_PASSWORD=app) and app/{api,auth}/Makefile's
# own DB_* defaults -- carrying no meaningful secret.
DB_COMPOSE_TOOLS := $(TOOLBOX_ENV_PREFIX) \
	$(COMPOSE_BIN) -f $(REPO_ROOT)/compose.yml -f $(COMPOSE_TOOLS_FILE)
DB_ONLINE_ENV := -e DB_HOST=postgres -e DB_PORT=5432 -e DB_USER=app -e DB_PASSWORD=app -e DB_SSLMODE=disable
ONLINE_DB := $(DB_COMPOSE_TOOLS) run --rm $(DB_ONLINE_ENV) tools make -C $(MODULE_DIR)

.PHONY: help
help: ## ターゲット一覧を表示する
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

ifdef IN_TOOLBOX
# ===========================================================================
# Inside the toolbox (IN_TOOLBOX=1, set by compose.tools.yml's
# x-tools-env anchor): the *-native targets are this Makefile's actual
# recipes, otherwise unchanged from pre-SPEC-009 behaviour.
# ===========================================================================

# goose (library dependency of this module -- go.mod's own
# `github.com/pressly/goose/v3` require is the single source of truth
# for the version actually built into the migrator binary; GOOSE_VERSION
# above, included from the repo root's versions.env, is a
# comment-only cross-reference kept in sync with it, not something this
# Makefile builds a command from -- see .claude/rules/db.md "版").
#
# goimports (GOIMPORTS_VERSION above, from versions.env) is baked into
# .devcontainer/toolchain and already on PATH via
# GOBIN=/usr/local/go-tools/bin -- called here by its installed binary
# name, not `go run pkg@version`: `go run` performs a module-proxy
# lookup on every invocation regardless of cache state and fails under
# this image's GOPROXY=off even with the exact module fully warmed (see
# .devcontainer/toolchain/README.md's "go run pkg@version does not work
# offline" note -- the SPEC-009 plan's own pre-approved fallback for
# this, now confirmed necessary rather than hypothetical).
GOIMPORTS := goimports

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

.PHONY: tidy-native
tidy-native: ## go.mod/go.sum を整理する (go mod tidy。check には含めない)
	go mod tidy

.PHONY: run-native
run-native: ## migrator を実行する (例: make run ARGS="-target api -migrations-dir ../api/infra/postgres/schema/migrations")
	go run ./cmd/migrator $(ARGS)

# Discrete DB_* settings for test-integration-native below, mirroring
# app/{api,auth}/Makefile's own defaults (ISSUE-016 review Major-1):
# these match the local compose Postgres and carry no meaningful
# secret; real environments (in particular CI) override every one of
# them via the environment (never edit them into this file). Used only
# when this -native target is invoked directly (e.g. inside an
# already-toolboxed shell without the DB_ONLINE_ENV overlay above) --
# the host-side `test-integration`/`run` targets below always pass
# DB_HOST=postgres explicitly via ONLINE_DB instead of relying on these.
DB_HOST ?= 127.0.0.1
DB_PORT ?= 5432
DB_USER ?= app
DB_PASSWORD ?= app
DB_SSLMODE ?= disable

.PHONY: test-integration-native
test-integration-native: ## infra/postgres の実 DB 統合テストを実行する (build tag integration。ISSUE-016 R-c のロール権限境界を実 Postgres に対して検証する。DB_* env で master 資格情報の接続先を指定)
	go test -tags=integration ./infra/postgres/...

else
# ===========================================================================
# On the host: re-invoke each target inside the pinned toolchain
# container. Command names are unchanged (SPEC-009 R4) -- only the
# execution environment moves.
# ===========================================================================

.PHONY: fmt
fmt: ## フォーマットを自動修正する (gofmt + goimports。toolbox 内 (tools-offline) で実行)
	$(OFFLINE) fmt-native

.PHONY: fmt-check
fmt-check: ## フォーマット差分がないかチェックする (toolbox 内 (tools-offline) で実行)
	$(OFFLINE) fmt-check-native

.PHONY: lint
lint: ## golangci-lint を実行する (toolbox 内 (tools-offline) で実行)
	$(OFFLINE) lint-native

.PHONY: vet
vet: ## go vet を実行する (toolbox 内 (tools-offline) で実行)
	$(OFFLINE) vet-native

.PHONY: build
build: ## 全パッケージをビルドする (toolbox 内 (tools-offline) で実行)
	$(OFFLINE) build-native

.PHONY: test
test: ## テストを実行する (toolbox 内 (tools-offline) で実行)
	$(OFFLINE) test-native

.PHONY: test-race
test-race: ## race detector 付きでテストを実行する (toolbox 内 (tools-offline) で実行)
	$(OFFLINE) test-race-native

.PHONY: check
check: ## 全チェックを実行する (fmt-check + lint + vet + build + test。toolbox 内 (tools-offline) で実行)
	$(OFFLINE) check-native

.PHONY: tidy
tidy: ## go.mod/go.sum を整理する (go mod tidy。ネットワーク有効な tools 経由。check には含めない)
	$(ONLINE) tidy-native

.PHONY: run
run: ## migrator を実行する (例: make run ARGS="-target api -migrations-dir ../api/infra/postgres/schema/migrations")。実 postgres 到達が要るため tools(+compose.yml) で実行する
	$(ONLINE_DB) run-native ARGS="$(ARGS)"

.PHONY: test-integration
test-integration: ## infra/postgres の実 DB 統合テストを実行する (build tag integration。実 postgres 到達が要るため tools(+compose.yml) で実行する)
	$(ONLINE_DB) test-integration-native

endif
