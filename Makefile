.DEFAULT_GOAL := help

# docker compose(プラグイン)があればそれを使い、無ければ standalone docker-compose にフォールバックする
COMPOSE := $(shell docker compose version >/dev/null 2>&1 && echo "docker compose" || echo "docker-compose")

.PHONY: help
help: ## ターゲット一覧を表示する (起動後: web http://localhost:8080 / api http://localhost:8081 / auth http://localhost:8082)
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-12s %s\n", $$1, $$2}'

.PHONY: build
build: ## 全サービスのイメージをビルドする
	$(COMPOSE) build

.PHONY: up
up: ## ビルドしてフォアグラウンドで起動する
	$(COMPOSE) up --build

.PHONY: up-d
up-d: ## ビルドしてデタッチ(バックグラウンド)で起動する
	$(COMPOSE) up -d --build

.PHONY: down
down: ## 全サービスを停止・コンテナを削除する
	$(COMPOSE) down

.PHONY: logs
logs: ## 全サービスのログを追従する
	$(COMPOSE) logs -f

.PHONY: ps
ps: ## 稼働状況を表示する
	$(COMPOSE) ps

.PHONY: restart
restart: ## 全サービスを再起動する
	$(COMPOSE) restart

.PHONY: clean
clean: ## 停止・コンテナ・volume を削除する
	$(COMPOSE) down -v

# ---------------------------------------------------------------------------
# AWS デプロイ(build-push)ツーリング(SPEC-004)
#
# 前提: app/iac/envs/dev が `terraform apply` 済みで、実行環境に AWS 認証情報
# (`aws configure` 等)が設定されていること。ECR リポジトリ URL・S3 バケット名・
# CloudFront distribution ID は `terraform output`(app/iac/envs/dev)から取得
# する — 参照する output 名の契約は app/iac/envs/dev/outputs.tf が正。
#
# apply 済み + AWS 認証情報が前提のため、agent はこれらのターゲットを実行
# しない(手動実行前提)。ARM64 を明示指定するのは、amd64 で誤ビルドすると
# ECS の ARM64 タスク定義でコンテナが起動できない不具合(ISSUE-014)を防ぐため。
# ---------------------------------------------------------------------------

AWS_REGION ?= ap-northeast-1
IMAGE_TAG ?= latest
TF_ENV_DIR := app/iac/envs/dev

.PHONY: push-images
push-images: ## api/auth イメージを ARM64 で ECR に build & push する(apply 済み + AWS 認証情報が前提。agent は実行しない・手動実行前提)
	@api_repo="$$(terraform -chdir=$(TF_ENV_DIR) output -raw api_ecr_repository_url)" && \
	auth_repo="$$(terraform -chdir=$(TF_ENV_DIR) output -raw auth_ecr_repository_url)" && \
	registry="$$(echo "$$api_repo" | cut -d/ -f1)" && \
	aws ecr get-login-password --region $(AWS_REGION) | docker login --username AWS --password-stdin "$$registry" && \
	docker buildx build --platform linux/arm64 --push -t "$$api_repo:$(IMAGE_TAG)" app/api && \
	docker buildx build --platform linux/arm64 --push -t "$$auth_repo:$(IMAGE_TAG)" app/auth

.PHONY: deploy-web
deploy-web: ## web を build して S3 sync + CloudFront invalidation する(apply 済み + AWS 認証情報が前提。agent は実行しない・手動実行前提)
	cd app/web && bun run build
	aws s3 sync app/web/dist "s3://$$(terraform -chdir=$(TF_ENV_DIR) output -raw web_bucket_name)" --delete
	aws cloudfront create-invalidation --distribution-id "$$(terraform -chdir=$(TF_ENV_DIR) output -raw cloudfront_distribution_id)" --paths '/*'
