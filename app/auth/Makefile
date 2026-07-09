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
GOOSE := go run github.com/pressly/goose/v3/cmd/goose@v3.24.1
GOOSE_DRIVER := postgres
GOOSE_DIR := db/migrations
# DSN assembled from the same discrete DB_* env vars the application
# itself uses (see infra/postgres/db.go); DB_SCHEMA is applied via
# search_path so goose's own version table (goose_db_version) lands in
# this stack's schema, not another stack's (plan §0 "スキーマ分離機構").
DB_HOST ?= 127.0.0.1
DB_PORT ?= 5432
DB_NAME ?= app
DB_USER ?= app
DB_PASSWORD ?= app
DB_SSLMODE ?= disable
DB_SCHEMA ?= auth
GOOSE_DBSTRING := postgres://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSLMODE)&search_path=$(DB_SCHEMA)

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

.PHONY: migrate-up
migrate-up: ## goose でマイグレーションを適用する (DB_* 環境変数で接続先を変更可)
	$(GOOSE) -dir $(GOOSE_DIR) $(GOOSE_DRIVER) "$(GOOSE_DBSTRING)" up

.PHONY: migrate-down
migrate-down: ## goose で直近のマイグレーションを 1 つ戻す
	$(GOOSE) -dir $(GOOSE_DIR) $(GOOSE_DRIVER) "$(GOOSE_DBSTRING)" down

.PHONY: migrate-status
migrate-status: ## goose の適用状況を表示する
	$(GOOSE) -dir $(GOOSE_DIR) $(GOOSE_DRIVER) "$(GOOSE_DBSTRING)" status

.PHONY: migrate-create
migrate-create: ## 新しい goose マイグレーションファイルを作成する (name=<slug> 必須)
	$(GOOSE) -dir $(GOOSE_DIR) create $(name) sql

.PHONY: test-integration
test-integration: ## infra/postgres の実 DB 統合テストを実行する (要: migrate-up 済みの Postgres, DB_* 環境変数)
	go test -tags=integration ./infra/postgres/...
