.DEFAULT_GOAL := help

GOIMPORTS := go run golang.org/x/tools/cmd/goimports@latest

# SPEC-005 2026-07-09 refactor (plan §RF.1.2/§RF.6.2 RF-e): this module
# runs goose as a *library* dependency (require in go.mod), unlike
# app/api and app/auth's Makefiles, which invoke it as a `go run
# pkg@version` CLI kept out of their go.mod entirely. GOOSE_VERSION
# here is a comment-only cross-reference, not a variable this Makefile
# actually uses -- go.mod's `require github.com/pressly/goose/v3` line
# is this module's single source of truth for the version actually
# built into the migrator binary. Keep the two in sync: bump go.mod
# (`go get github.com/pressly/goose/v3@<new version>`) and this
# comment together, and bump app/{api,auth}/Makefile's own
# GOOSE_VERSION (used only by their migrate-create CLI target) to
# match, so every goose invocation across the repo stays on one
# version.
GOOSE_VERSION := v3.24.1 # must match go.mod's github.com/pressly/goose/v3 require and app/{api,auth}/Makefile's GOOSE_VERSION

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
run: ## migrator を実行する (例: make run ARGS="-target api -migrations-dir ../api/db/migrations")
	go run ./cmd/migrator $(ARGS)
