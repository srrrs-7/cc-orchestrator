.DEFAULT_GOAL := help

GOIMPORTS := go run golang.org/x/tools/cmd/goimports@latest

# swag v2 is still pre-1.0 (RC); pin the exact tag here so `make openapi`
# is reproducible. `go run pkg@version` builds/runs the tool without
# touching this module's go.mod/go.sum (std-lib runtime is preserved).
SWAG := go run github.com/swaggo/swag/v2/cmd/swag@v2.0.0-rc5

# SPEC-005: goose (migrations) and sqlc (query codegen) are run as
# `go run pkg@version` CLIs -- like SWAG above -- so this module's
# go.mod/go.sum only ever gains the Postgres driver (pgx) as a runtime
# dependency, never the tool binaries themselves. This Makefile is the
# single source of truth for the pinned versions
# (docs/plans/SPEC-005-plan.md §0 "版固定").
#
# sqlc v1.28.0 (the plan's baseline) fails to build from source on this
# machine's toolchain (macOS SDK / Xcode clang rejects a `strchrnul`
# redeclaration inside the vendored pg_query_go/v5 C parser -- a known
# upstream issue on newer macOS SDKs, fixed once sqlc moved to
# pg_query_go/v6). v1.31.1 is the nearest stable release that resolves
# and builds cleanly here; it is used in place of the plan's baseline
# per the plan's own escape hatch ("resolve 不可なら最寄りの stable patch に
# 調整して Makefile に確定記載する"). goose v3.24.1 (the plan's baseline)
# builds without issue and is used as-is.
GOOSE_VERSION := v3.24.1
SQLC_VERSION := v1.31.1

# goose is a library dependency of app/migrator (2026-07-09 refactor,
# SPEC-005 plan §RF.1.2/§RF.6.2 RF-e), not a runtime require of this
# module; the `go run` CLI below is used only by migrate-create, which
# just scaffolds a new migration file (no DB connection). Migration
# *application* (up/down/status) now goes through app/migrator, not
# this Makefile -- see the root Makefile's `migrate` target and
# .claude/rules/db.md. Keep GOOSE_VERSION in sync with
# app/migrator/go.mod's github.com/pressly/goose/v3 require.
GOOSE := go run github.com/pressly/goose/v3/cmd/goose@$(GOOSE_VERSION)
SQLC := go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)

MIGRATIONS_DIR := db/migrations

# Discrete DB_* settings (SPEC-005 R6), documenting the same env
# contract infra/postgres/db.go's ConfigFromEnv and app/migrator read
# (DB_NAME defaults to "api", this stack's own dedicated Postgres
# database -- 2026-07-09 refactor, plan §RF.1.1: api/auth are separated
# by database, not by schema/search_path, so there is no DB_SCHEMA).
# These defaults match the local compose Postgres (impl-db's
# compose.yml) and carry no meaningful secret; real environments
# override every one of them via the environment (never edit them into
# this file). test-integration relies on these being exported in the
# invoking shell (this Makefile does not forward them itself).
DB_HOST ?= 127.0.0.1
DB_PORT ?= 5432
DB_NAME ?= api
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
run: ## API サーバーを起動する (PORT 環境変数でポート変更可)
	go run ./cmd/api

.PHONY: openapi
openapi: ## OpenAPI 3.1 契約 (docs/openapi.yaml) を swag v2 注釈から生成する (check には含めない)
	$(SWAG) init -g cmd/api/main.go -o docs -ot yaml --v3.1
	mv docs/swagger.yaml docs/openapi.yaml

.PHONY: sqlc
sqlc: ## sqlc でクエリ (db/queries) からコード (infra/postgres/sqlcgen) を生成する (生成であり検査ではないため check には含めない)
	$(SQLC) generate

.PHONY: migrate-create
migrate-create: ## 新しいマイグレーションファイルを作成する (例: make migrate-create name=add_foo_column)
	@if [ -z "$(name)" ]; then echo "usage: make migrate-create name=<slug>"; exit 1; fi
	$(GOOSE) -dir $(MIGRATIONS_DIR) create $(name) sql

.PHONY: test-integration
test-integration: ## infra/postgres の実 DB 統合テストを実行する (build tag integration。事前に app/migrator で DB 作成 + up 適用が必要。DB_* env で接続先を指定)
	go test -tags=integration ./infra/postgres/...
