.DEFAULT_GOAL := help

GOIMPORTS := go run golang.org/x/tools/cmd/goimports@latest

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
