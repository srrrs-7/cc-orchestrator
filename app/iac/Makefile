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
#
# --- SPEC-009 Phase B: transparent toolbox delegation -----------------------
#
# 公開ターゲット名・挙動(上記の表・.claude/rules/iac.md の「コマンド」表)は
# SPEC-009 前と不変(R4)。実体だけが docker/toolchain の単一 polyglot イメージ
# (compose.tools.yml 経由)の中で動くようになり、ホストに terraform/tflint/trivy
# が入っていなくてもこの Makefile と docker だけで動く。実際のレシピは
# `ifdef IN_TOOLBOX` 配下の `<target>-native` に退避してあり、`IN_TOOLBOX` は
# compose.tools.yml の tools/tools-offline サービスの environment (x-tools-env)
# で常時 1 が設定されているため、コンテナ内で誤って二重に docker を起動する
# ことはない(app/api・app/auth・app/migrator の各 Makefile と同型のパターン。
# 詳細は docs/plans/SPEC-009-plan.md「Makefile 透過ラッパーの形」)。
#
# terraform 固有の network 割り当て(compose.tools.yml の tools(network 有) /
# tools-offline(network none) のどちらに振るか)は、SPEC-009 plan 冒頭案から
# 実地検証(このコミットの実装過程)で以下のとおり確定させた。判断根拠は
# 実装報告にも記載:
#   - fmt / fmt-check: ファイル整形のみで provider 等を一切参照しない。
#     tools-offline (network none) で完結することを確認済み。
#   - lint (tflint --recursive): このリポジトリには .tflint.hcl が無く
#     ruleset プラグインを使っていない(コア組み込みルールのみ)ため、
#     tools-offline で完結することを確認済み。将来 ruleset プラグインを
#     追加する場合、`tflint --init` がプラグイン取得に network を要する
#     可能性がある点に注意(今回は未使用のため対象外)。
#   - security (trivy config): 既定では毎回 checks-bundle の更新を
#     mirror.gcr.io から試みるため、tools-offline (network none) 下では
#     取得が失敗しても embedded checks に自動フォールバックして完走は
#     するが、毎回無駄な待ち時間が発生し、かつ結果が「取得できた場合/
#     できなかった場合」で非決定的になる(実地確認済み)。
#     `--skip-check-update` を付けてこの取得試行そのものを止め、
#     tools-offline 上で常に embedded checks を使う決定論的な動作にした
#     (直接コマンドは `trivy config .` だが、実体はこのフラグ付きで
#     呼ぶ。.claude/rules/iac.md の「直接コマンド」列との差分は報告済み)。
#   - init-local / validate: provider (lock file に記載の無いプラットフォーム
#     分を含む) の取得に registry.terraform.io への到達が要る(実地確認:
#     `terraform init -backend=false` は network none 下では失敗する)。
#     `validate` は `init-local` に依存する既存の Make 依存関係を単一の
#     コンテナ呼び出しで完結させたいため、validate 自体は provider 取得済み
#     なら network 不要であることを確認した上で、あえて分割せず tools
#     (network 有) にまとめて寄せている。
#   - init (実 backend) / plan / apply: 実 backend (S3 state) と AWS
#     provider への到達が要るため tools (network 有)。加えて
#     compose.tools.yml はホストの ~/.aws 等シークレットファイルを一切
#     マウントしない設計のため、AWS 認証情報は環境変数
#     (AWS_ACCESS_KEY_ID 等。下記 AWS_ENV_PASSTHROUGH)でのみ透過する。
#     プロファイルファイル方式の認証はコンテナ内では機能しない
#     (人間が plan/apply/init を実行する前に、セッション認証情報を
#     環境変数へ export しておくこと)。
#
# `check` (fmt-check -> validate -> lint -> security) は他スタックの
# Makefile と異なり、単一の `<service> make -C app/iac check-native` には
# 委譲しない: validate だけ network が要り、他 3 つは network を要さない
# ため、1 回のコンテナ起動(1 サービス)にまとめると network 最小化の原則
# (SPEC-009 の多層防御方針)を壊してしまう。そのためホスト側の `check` は
# 純粋な前提ターゲットの並び(`fmt-check validate lint security`)のままにし、
# 各ターゲットが個別に適切な service へ委譲する形にしている
# (`check-native` は用意していない: network none 下で呼ぶと validate-native
# の provider 取得が失敗するため、単体では意味を持たない)。
# -----------------------------------------------------------------------------

.DEFAULT_GOAL := help

REPO_ROOT := $(shell git rev-parse --show-toplevel)
# versions.env はこの Makefile 自身では値を消費しない(terraform/tflint/trivy
# の版は docker/toolchain/Dockerfile が build ARG として焼き込む側の関心事で、
# ここではバイナリを名前で呼ぶだけ)。それでも単一ソースの所在を明示する
# ドキュメントとして include しておく(他スタックの Makefile と同じ扱い)。
include $(REPO_ROOT)/versions.env

MODULE_DIR := app/iac

# 環境のパラメータ化。将来 envs/prod 等を追加しても
# `make <target> ENV=prod` でそのまま動く。
ENV ?= dev
ENV_DIR := envs/$(ENV)

# docker compose (プラグイン) があればそれを使い、無ければ standalone
# docker-compose にフォールバックする(root Makefile / app/migrator/Makefile
# と同じ probe。docker/toolchain/README.md もこのフォールバックを前提に
# している)。
COMPOSE_BIN := $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")

# TOOLBOX_UID/TOOLBOX_GID は `-e`/`docker compose run -e` ではなく実プロセスの
# 環境変数として渡す必要がある: compose.tools.yml の
# `user: "${TOOLBOX_UID:-1000}:${TOOLBOX_GID:-1000}"` は compose 自身が
# コンテナ生成前に自プロセス環境から `${...}` を解決する値であり、`-e` は
# コンテナ生成後にしか効かず間に合わない(同ファイルの header comment 参照)。
COMPOSE_TOOLS := TOOLBOX_UID=$(shell id -u) TOOLBOX_GID=$(shell id -g) \
	$(COMPOSE_BIN) --env-file $(REPO_ROOT)/versions.env -f $(REPO_ROOT)/compose.tools.yml

# 実 backend / AWS provider を使う init(非 --backend=false) / plan / apply
# 向け。compose.tools.yml はホストの ~/.aws 等を一切マウントしないため、
# ホスト側で環境変数としてエクスポート済みの AWS 認証情報だけをそのまま
# コンテナへ透過する(`-e VAR` は値省略時、docker がホスト側シェル環境の
# 値をそのまま引き継ぐ挙動を利用している。値そのものを Makefile の
# コマンドライン引数に載せないための選択)。
AWS_ENV_PASSTHROUGH := -e AWS_ACCESS_KEY_ID -e AWS_SECRET_ACCESS_KEY -e AWS_SESSION_TOKEN \
	-e AWS_REGION -e AWS_DEFAULT_REGION -e AWS_PROFILE

# tools-offline (network_mode:none): fmt/fmt-check/lint/security の既定 service
# (上のヘッダコメントの network 割り当て判断を参照)。
OFFLINE := $(COMPOSE_TOOLS) run --rm tools-offline make -C $(MODULE_DIR)
# tools (network enabled): init-local/validate 用(AWS 認証情報は不要)。
ONLINE := $(COMPOSE_TOOLS) run --rm tools make -C $(MODULE_DIR)
# tools (network enabled) + AWS 認証情報の透過: init(実backend)/plan/apply 用。
ONLINE_AWS := $(COMPOSE_TOOLS) run --rm $(AWS_ENV_PASSTHROUGH) tools make -C $(MODULE_DIR)

.PHONY: help
help: ## このヘルプを表示する (デフォルトゴール)
	@echo "Usage: make <target> [ENV=dev|prod|...]  (default ENV=dev)"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / && !/-native:/ {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)
	@echo ""
	@echo "Current: ENV=$(ENV) (target directory: $(ENV_DIR))"

ifdef IN_TOOLBOX
# =============================================================================
# コンテナ内 (IN_TOOLBOX=1、compose.tools.yml の x-tools-env が常時設定)。
# 実体はここだけにあり、再委譲は起きない。
# =============================================================================

.PHONY: guard-env-native
## 対象環境ディレクトリの存在確認。存在しない ENV を指定した場合はここで
## 分かりやすく失敗させる (他ターゲットの前提として使う)。
guard-env-native:
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

.PHONY: init-native
init-native: guard-env-native ## terraform init (実backend。実利用向け。事前に S3 backend 等の実値設定と認証情報が必要)
	terraform -chdir=$(ENV_DIR) init

.PHONY: init-local-native
init-local-native: guard-env-native ## terraform init -backend=false (認証情報・backend 不要。validate/lint/CI 向け)
	terraform -chdir=$(ENV_DIR) init -backend=false

.PHONY: fmt-native
fmt-native: ## terraform fmt -recursive (自動修正。ENV非依存、app/iac ルート全体 = modules + envs が対象)
	terraform fmt -recursive

.PHONY: fmt-check-native
fmt-check-native: ## terraform fmt -check -recursive (チェックのみ、書き換えない。ENV非依存、app/iac ルート全体が対象)
	terraform fmt -check -recursive

# validate は事前に init (provider/module の解決) が必要。envs/dev の backend は
# 認証情報なしでは init できない S3 プレースホルダのため、実 backend を使う init
# ではなく init-local に依存させている。
.PHONY: validate-native
validate-native: init-local-native ## terraform validate (前提: init-local。このターゲットが自動で実行する)
	terraform -chdir=$(ENV_DIR) validate

.PHONY: lint-native
lint-native: guard-env-native ## tflint --recursive (対象ディレクトリで実行)
	@if ! command -v tflint >/dev/null 2>&1; then \
		echo "Error: tflint is not installed. See https://github.com/terraform-linters/tflint#installation"; \
		exit 1; \
	fi
	cd $(ENV_DIR) && tflint --recursive

# --skip-check-update: checks-bundle の更新取得を試みない (tools-offline =
# network none 下でも遅延なく embedded checks で決定論的に完走させる。
# ファイル冒頭のコメント参照)。
.PHONY: security-native
security-native: guard-env-native ## trivy config . (対象ディレクトリで実行。別名: scan)
	@if ! command -v trivy >/dev/null 2>&1; then \
		echo "Error: trivy is not installed. See https://aquasecurity.github.io/trivy/latest/getting-started/installation/"; \
		exit 1; \
	fi
	cd $(ENV_DIR) && trivy config --skip-check-update .

.PHONY: scan-native
scan-native: security-native ## security の別名

.PHONY: plan-native
plan-native: guard-env-native ## terraform plan (差分確認。実 backend の init と認証情報が前提)
	terraform -chdir=$(ENV_DIR) plan

# apply は agent からは実行しない (.claude/rules/iac.md の方針)。
# plan の結果を確認したうえで、人間が判断して手動で実行すること。
.PHONY: apply-native
apply-native: guard-env-native ## terraform apply -- agent は実行禁止。plan結果を確認のうえ人間が手動実行すること
	@echo "===================================================================="
	@echo "  WARNING: 'terraform apply' changes real infrastructure."
	@echo "  Review 'make plan ENV=$(ENV)' output first."
	@echo "  This must be run manually by a human, never by an agent."
	@echo "===================================================================="
	terraform -chdir=$(ENV_DIR) apply

else
# =============================================================================
# ホスト側: docker/toolchain の単一 polyglot イメージへ透過委譲する
# (SPEC-009)。ホストに terraform / tflint / trivy が入っていなくても、
# この Makefile と docker だけで動く。
# =============================================================================

.PHONY: init
init: ## terraform init (実backend。実利用向け。事前に S3 backend 等の実値設定と認証情報が必要)
	$(ONLINE_AWS) init-native ENV=$(ENV)

.PHONY: init-local
init-local: ## terraform init -backend=false (認証情報・backend 不要。validate/lint/CI 向け)
	$(ONLINE) init-local-native ENV=$(ENV)

.PHONY: fmt
fmt: ## terraform fmt -recursive (自動修正。ENV非依存、app/iac ルート全体 = modules + envs が対象)
	$(OFFLINE) fmt-native

.PHONY: fmt-check
fmt-check: ## terraform fmt -check -recursive (チェックのみ、書き換えない。ENV非依存、app/iac ルート全体が対象)
	$(OFFLINE) fmt-check-native

.PHONY: validate
validate: ## terraform validate (前提: init-local。このターゲットが自動で実行する)
	$(ONLINE) validate-native ENV=$(ENV)

.PHONY: lint
lint: ## tflint --recursive (対象ディレクトリで実行)
	$(OFFLINE) lint-native ENV=$(ENV)

.PHONY: security
security: ## trivy config . (対象ディレクトリで実行。別名: scan)
	$(OFFLINE) security-native ENV=$(ENV)

.PHONY: scan
scan: security ## security の別名

.PHONY: plan
plan: ## terraform plan (差分確認。実 backend の init と認証情報が前提)
	$(ONLINE_AWS) plan-native ENV=$(ENV)

.PHONY: check
check: fmt-check validate lint security ## fmt-check -> validate -> lint -> security を一括実行 (checker 向け。validate だけ network 有・他は network none で個別に実行する)

# apply は agent からは実行しない (.claude/rules/iac.md の方針)。
# plan の結果を確認したうえで、人間が判断して手動で実行すること。
.PHONY: apply
apply: ## terraform apply -- agent は実行禁止。plan結果を確認のうえ人間が手動実行すること
	$(ONLINE_AWS) apply-native ENV=$(ENV)

endif
