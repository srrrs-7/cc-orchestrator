.DEFAULT_GOAL := help

# --- SPEC-009: transparent toolbox delegation (container-only execution) ------
REPO_ROOT := $(shell git rev-parse --show-toplevel)
DEVCONTAINER_DIR := $(REPO_ROOT)/.devcontainer
COMPOSE_TOOLS_FILE := $(DEVCONTAINER_DIR)/compose.tools.yml
include $(DEVCONTAINER_DIR)/versions.env
export GO_VERSION BUN_VERSION GOLANGCI_LINT_VERSION TERRAFORM_VERSION TFLINT_VERSION TRIVY_VERSION SQLC_VERSION GOOSE_VERSION SWAG_VERSION GOIMPORTS_VERSION POSTGRES_VERSION

TOOLBOX_ENV := COMPOSE_PROJECT_NAME=cc-orchestrator TOOLBOX_UID="$$(id -u)" TOOLBOX_GID="$$(id -g)" \
	TOOLBOX_WORKSPACE="$(REPO_ROOT)" TOOLBOX_CONTEXT="$(DEVCONTAINER_DIR)/toolchain"

COMPOSE_BIN := $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")
COMPOSE    := $(COMPOSE_BIN) -f $(COMPOSE_TOOLS_FILE)
WORKDIR    := --workdir /workspace/app/auth-web

OFFLINE := $(TOOLBOX_ENV) $(COMPOSE) run --rm $(WORKDIR) tools-offline
ONLINE  := $(TOOLBOX_ENV) $(COMPOSE) run --rm $(WORKDIR) tools

ifdef IN_TOOLBOX
HELP_FILTER := grep -E -- '-native:'
else
HELP_FILTER := grep -v -E -- '-native:'
endif

.PHONY: help
help: ## ターゲット一覧を表示する
	@grep -E '^[a-zA-Z0-9_.-]+:.*?## ' $(MAKEFILE_LIST) | $(HELP_FILTER) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-16s %s\n", $$1, $$2}'

ifdef IN_TOOLBOX

.PHONY: install-native
install-native: ## 依存をインストールする (INSTALL_FLAGS=--frozen-lockfile で CI 固定)
	bun install $(INSTALL_FLAGS)

.PHONY: format-check-native
format-check-native: ## Biome format チェック (package.json format:check)
	bun run format:check

.PHONY: format-native
format-native: ## Biome format 自動修正 (package.json format)
	bun run format

.PHONY: lint-native
lint-native: ## Biome lint (package.json lint)
	bun run lint

.PHONY: typecheck-native
typecheck-native: ## tsc --noEmit (package.json typecheck)
	bun run typecheck

.PHONY: test-native
test-native: ## Vitest (package.json test; 単一ファイルは TEST=<path> を渡す)
	bun run test $(TEST)

.PHONY: build-native
build-native: ## tsc + vite build (package.json build)
	bun run build

.PHONY: check-native
check-native: format-check-native lint-native typecheck-native test-native build-native ## 全チェック (format-check + lint + typecheck + test + build)

else

.PHONY: install
install: ## 依存をインストールする (INSTALL_FLAGS=--frozen-lockfile で CI 固定)
	$(ONLINE) make install-native INSTALL_FLAGS="$(INSTALL_FLAGS)"

.PHONY: format-check
format-check: ## Biome format チェック
	$(OFFLINE) make format-check-native

.PHONY: format
format: ## Biome format 自動修正
	$(OFFLINE) make format-native

.PHONY: lint
lint: ## Biome lint
	$(OFFLINE) make lint-native

.PHONY: typecheck
typecheck: ## tsc --noEmit
	$(OFFLINE) make typecheck-native

.PHONY: test
test: ## Vitest (単一ファイルは TEST=<path> を渡す)
	$(OFFLINE) make test-native TEST="$(TEST)"

.PHONY: build
build: ## tsc + vite build
	$(OFFLINE) make build-native

.PHONY: check
check: ## 全チェック (format-check + lint + typecheck + test + build)
	$(OFFLINE) make check-native

endif
