.DEFAULT_GOAL := help

GOIMPORTS := go run golang.org/x/tools/cmd/goimports@latest

# SPEC-005: goose (migrations) and sqlc (query -> Go codegen) are
# invoked via `go run pkg@version` (like `make openapi`'s swag), so
# they never become runtime dependencies of this module's go.mod/
# go.sum. Versions here are the single source of truth (plan §0 "版固
# 定": pinned as a baseline by planner, confirmed resolvable by
# impl-db at implementation time -- adjust here, and only here, if a
# version ever needs to move).
#
# sqlc v1.28.0 (the plan's baseline) fails to build from source on
# macOS toolchains where the SDK/Xcode clang rejects a `strchrnul`
# redeclaration inside the vendored pg_query_go/v5 C parser (a known
# upstream issue, fixed once sqlc moved to pg_query_go/v6). app/api
# hit the same wall and pinned v1.31.1 instead; this module is
# unified onto the same version so both stacks share one sqlc CLI
# (single toolchain to reason about, no per-stack CGO_CFLAGS
# workaround needed).
SQLC := go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1
# goose is a library dependency of app/migrator (2026-07-09 refactor,
# SPEC-005 plan §RF.1.2/§RF.6.2 RF-e), not a runtime require of this
# module; the `go run` CLI below is used only by migrate-create, which
# just scaffolds a new migration file (no DB connection). Migration
# *application* (up/down/status) now goes through app/migrator, not
# this Makefile -- see the root Makefile's `migrate` target and
# .claude/rules/db.md. Keep this version in sync with
# app/migrator/go.mod's github.com/pressly/goose/v3 require.
GOOSE := go run github.com/pressly/goose/v3/cmd/goose@v3.24.1
GOOSE_DIR := db/migrations
# Discrete DB_* settings (SPEC-005 R6), documenting the same env
# contract infra/postgres/db.go's ConfigFromEnv and app/migrator read
# (DB_NAME defaults to "auth", this stack's own dedicated Postgres
# database -- 2026-07-09 refactor, plan §RF.1.1: api/auth are separated
# by database, not by schema/search_path, so there is no DB_SCHEMA).
# These defaults match the local compose Postgres (impl-db's
# compose.yml) and carry no meaningful secret; real environments
# override every one of them via the environment (never edit them into
# this file). test-integration relies on these being exported in the
# invoking shell (this Makefile does not forward them itself).
DB_HOST ?= 127.0.0.1
DB_PORT ?= 5432
DB_NAME ?= auth
DB_USER ?= app
DB_PASSWORD ?= app
DB_SSLMODE ?= disable

.PHONY: help
help: ## ターゲット一覧を表示する
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

.PHONY: fmt
fmt: ## フォーマットを自動修正する (gofmt + goimports)
	gofmt -w .
	$(GOIMPORTS) -w .

.PHONY: fmt-check
fmt-check: ## フォーマット差分がないかチェックする
	@files="$$(gofmt -l .)"; \
	if [ -n "$$files" ]; then \
		echo "gofmt required:"; echo "$$files"; exit 1; \
	fi

.PHONY: lint
lint: ## golangci-lint を実行する
	golangci-lint run ./...

.PHONY: vet
vet: ## go vet を実行する
	go vet ./...

.PHONY: build
build: ## 全パッケージをビルドする
	go build ./...

.PHONY: test
test: ## テストを実行する
	go test ./...

.PHONY: test-race
test-race: ## race detector 付きでテストを実行する
	go test -race ./...

.PHONY: check
check: fmt-check lint vet build test ## 全チェックを実行する (fmt-check + lint + vet + build + test)

.PHONY: run
run: ## 認可サーバーを起動する (PORT / ISSUER 環境変数で変更可)
	go run ./cmd/authz

.PHONY: sqlc
sqlc: ## db/queries から sqlcgen (infra/postgres/sqlcgen) を生成する (check には含めない。生成後は commit する)
	$(SQLC) generate

.PHONY: migrate-create
migrate-create: ## 新しい goose マイグレーションファイルを作成する (name=<slug> 必須)
	$(GOOSE) -dir $(GOOSE_DIR) create $(name) sql

.PHONY: test-integration
test-integration: ## infra/postgres の実 DB 統合テストを実行する (要: app/migrator で DB 作成 + up 適用済みの Postgres, DB_* 環境変数)
	go test -tags=integration ./infra/postgres/...
