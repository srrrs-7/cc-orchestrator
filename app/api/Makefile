.DEFAULT_GOAL := help

GOIMPORTS := go run golang.org/x/tools/cmd/goimports@latest

# swag v2 is still pre-1.0 (RC); pin the exact tag here so `make openapi`
# is reproducible. `go run pkg@version` builds/runs the tool without
# touching this module's go.mod/go.sum (std-lib runtime is preserved).
SWAG := go run github.com/swaggo/swag/v2/cmd/swag@v2.0.0-rc5

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
