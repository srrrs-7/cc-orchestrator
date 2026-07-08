# app/iac/Makefile
#
# Terraform 汎用コマンド集約。
# コマンドの正は .claude/rules/iac.md の「コマンド」表であり、このファイルは
# その表を make ターゲットとしてラップしたものにすぎない
# (fmt/fmt-check は app/iac ルート全体、それ以外は envs/<env> が対象)。
# 表を更新したら必ずこのファイルも合わせる。
#
# 使い方:
#   make            # help を表示 (デフォルトゴール)
#   make plan        # ENV=dev で terraform plan
#   make plan ENV=prod
#   make check       # fmt-check -> validate -> lint -> security を一括実行 (checker 向け)
#
# 実行場所には 2 種類ある:
#   - fmt / fmt-check: env に依存せず app/iac ルート全体 (modules + envs) を対象に実行する。
#     terraform fmt はディレクトリ構造・モジュール解決に依存せず全 .tf を整形するだけなので、
#     env ディレクトリ基点にすると modules/ が対象から外れてしまう。
#   - それ以外 (init / init-local / validate / lint / security / plan / apply):
#     envs/$(ENV) を対象ディレクトリとして実行する (terraform は -chdir、
#     tflint / trivy は cd してから実行する)。モジュール解決や env 単位の検証が必要なため。

.DEFAULT_GOAL := help

# 環境のパラメータ化。将来 envs/prod 等を追加しても
# `make <target> ENV=prod` でそのまま動く。
ENV ?= dev
ENV_DIR := envs/$(ENV)

.PHONY: help guard-env init init-local fmt fmt-check validate lint security scan plan check apply

## 対象環境ディレクトリの存在確認。存在しない ENV を指定した場合はここで
## 分かりやすく失敗させる (他ターゲットの前提として使う)。
guard-env:
	@if [ ! -d "$(ENV_DIR)" ]; then \
		echo "Error: environment directory '$(ENV_DIR)' not found."; \
		echo "Available environments:"; \
		found=0; \
		for d in $$(find envs -mindepth 1 -maxdepth 1 -type d 2>/dev/null); do \
			echo "  - $$(basename $$d)"; \
			found=1; \
		done; \
		if [ "$$found" = "0" ]; then echo "  (none found under envs/)"; fi; \
		exit 1; \
	fi

help: ## このヘルプを表示する (デフォルトゴール)
	@echo "Usage: make <target> [ENV=dev|prod|...]  (default ENV=dev)"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "Current: ENV=$(ENV) (target directory: $(ENV_DIR))"

init: guard-env ## terraform init (実backend。実利用向け。事前に S3 backend 等の実値設定と認証情報が必要)
	terraform -chdir=$(ENV_DIR) init

init-local: guard-env ## terraform init -backend=false (認証情報・backend 不要。validate/lint/CI 向け)
	terraform -chdir=$(ENV_DIR) init -backend=false

fmt: ## terraform fmt -recursive (自動修正。ENV非依存、app/iac ルート全体 = modules + envs が対象)
	terraform fmt -recursive

fmt-check: ## terraform fmt -check -recursive (チェックのみ、書き換えない。ENV非依存、app/iac ルート全体が対象)
	terraform fmt -check -recursive

# validate は事前に init (provider/module の解決) が必要。envs/dev の backend は
# 認証情報なしでは init できない S3 プレースホルダのため、実 backend を使う init
# ではなく init-local に依存させている。
validate: init-local ## terraform validate (前提: init-local。このターゲットが自動で実行する)
	terraform -chdir=$(ENV_DIR) validate

lint: guard-env ## tflint --recursive (対象ディレクトリで実行)
	@if ! command -v tflint >/dev/null 2>&1; then \
		echo "Error: tflint is not installed. See https://github.com/terraform-linters/tflint#installation"; \
		exit 1; \
	fi
	cd $(ENV_DIR) && tflint --recursive

security: guard-env ## trivy config . (対象ディレクトリで実行。別名: scan)
	@if ! command -v trivy >/dev/null 2>&1; then \
		echo "Error: trivy is not installed. See https://aquasecurity.github.io/trivy/latest/getting-started/installation/"; \
		exit 1; \
	fi
	cd $(ENV_DIR) && trivy config .

scan: security ## security の別名

plan: guard-env ## terraform plan (差分確認。実 backend の init と認証情報が前提)
	terraform -chdir=$(ENV_DIR) plan

check: fmt-check validate lint security ## fmt-check -> validate -> lint -> security を一括実行 (checker 向け)

# apply は agent からは実行しない (.claude/rules/iac.md の方針)。
# plan の結果を確認したうえで、人間が判断して手動で実行すること。
apply: guard-env ## terraform apply -- agent は実行禁止。plan結果を確認のうえ人間が手動実行すること
	@echo "===================================================================="
	@echo "  WARNING: 'terraform apply' changes real infrastructure."
	@echo "  Review 'make plan ENV=$(ENV)' output first."
	@echo "  This must be run manually by a human, never by an agent."
	@echo "===================================================================="
	terraform -chdir=$(ENV_DIR) apply
