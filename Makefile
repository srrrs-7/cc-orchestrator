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
up: migrate ## postgres を起動・マイグレーション適用した上でビルドしてフォアグラウンドで起動する
	$(COMPOSE) up --build

.PHONY: up-d
up-d: migrate ## postgres を起動・マイグレーション適用した上でビルドしてデタッチ(バックグラウンド)で起動する
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
# ローカル Postgres(SPEC-005。2026-07-09 リファクタ: 別データベース + app/migrator)
#
# api/auth はどちらも起動時に DB_HOST の有無で永続化先を選ぶ fail-closed な
# 配線になっている(app/{api,auth}/infra/postgres/db.go の SelectMode)。
# compose.yml は api/auth に DB_HOST=postgres 他の DB_* を注入済みのため、
# `up`/`up-d` は必ず Postgres 経路を使う(memory 経路には戻らない)。
# マイグレーション未適用のテーブル欠如でクラッシュループしないよう、
# `up`/`up-d` は `migrate`(→ `db-up`)を前提ターゲットにしている。
#
# api・auth は同一 Postgres インスタンス上の別データベース("api"/"auth")に
# 分離されている(旧: 単一 database + search_path 別スキーマ)。`migrate` は
# 共通の `app/migrator`(`app/migrator/main.go`)を `-target` 違いで 2 回実行し、
# 各データベースを(未存在なら)作成した上で当該スタックの db/migrations を
# 適用する。DB_NAME は migrator の既定(-target と同名: api/auth)に任せ、
# 接続先(host/port/user/password/sslmode)だけをローカル compose の postgres
# に合わせて明示する(app/api・app/auth Makefile の DB_* 既定と同じ値)。
#
# `go run ./app/migrator` はこの実行にホストの Go ツールチェーンを要求する
# (app/api・app/auth の `make check` 等と同じ前提)。docker のみで完結しない
# 点に注意。
# ---------------------------------------------------------------------------

.PHONY: db-up
db-up: ## postgres のみを起動し healthy になるまで待つ
	$(COMPOSE) up -d --wait postgres

MIGRATOR_DB_ENV := DB_HOST=127.0.0.1 DB_PORT=5432 DB_USER=app DB_PASSWORD=app DB_SSLMODE=disable

# `go run ./app/migrator` cannot be invoked from this (root) directory:
# app/migrator has its own go.mod (an independent module, deliberately
# not part of a workspace with app/api/app/auth -- plan §RF.1.2/§RF.1.3
# "goose の閉じ込め"), and this repo root has no go.mod of its own for
# Go to resolve a "main module" from. `go -C app/migrator run .` runs
# in app/migrator's own module context instead; -migrations-dir is
# passed as an absolute path ($(CURDIR)-rooted) so it resolves
# correctly regardless of that directory change.
.PHONY: migrate
migrate: db-up ## api/auth のデータベースをローカル compose の postgres に作成・マイグレーション適用する (app/migrator 経由。db-up を前提として実行)
	$(MIGRATOR_DB_ENV) go -C app/migrator run . -target api -migrations-dir $(CURDIR)/app/api/db/migrations
	$(MIGRATOR_DB_ENV) go -C app/migrator run . -target auth -migrations-dir $(CURDIR)/app/auth/db/migrations

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
