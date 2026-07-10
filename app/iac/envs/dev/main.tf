locals {
  name_prefix = "${var.project}-${var.environment}"

  common_tags = {
    Project     = var.project
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}

# Single generation point for the CloudFront <-> ALB origin-verify secret,
# shared between the cdn module and every modules/service instance (see
# modules/service and modules/cdn README for the two-layer defense this
# implements, R3). Alphanumeric-only to avoid characters that are invalid in
# HTTP header values.
resource "random_password" "origin_verify" {
  length  = 32
  special = false
}

module "network" {
  source = "../../modules/network"

  name_prefix          = local.name_prefix
  vpc_cidr             = var.vpc_cidr
  azs                  = var.azs
  public_subnet_cidrs  = var.public_subnet_cidrs
  private_subnet_cidrs = var.private_subnet_cidrs
  container_port       = var.container_port
  db_port              = var.db_port
  tags                 = local.common_tags
}

module "db" {
  source = "../../modules/db"

  name_prefix             = local.name_prefix
  private_subnet_ids      = module.network.private_subnet_ids
  rds_sg_id               = module.network.rds_sg_id
  instance_class          = var.db_instance_class
  allocated_storage       = var.db_allocated_storage
  engine_version          = var.db_engine_version
  db_name                 = var.db_name
  master_username         = var.db_master_username
  multi_az                = var.db_multi_az
  deletion_protection     = var.db_deletion_protection
  skip_final_snapshot     = var.db_skip_final_snapshot
  backup_retention_period = var.db_backup_retention_period
  api_app_role_name       = var.db_api_app_role_name
  auth_app_role_name      = var.db_auth_app_role_name
  tags                    = local.common_tags
}

module "platform" {
  source = "../../modules/platform"

  name_prefix       = local.name_prefix
  vpc_id            = module.network.vpc_id
  public_subnet_ids = module.network.public_subnet_ids
  alb_sg_id         = module.network.alb_sg_id

  tags = local.common_tags
}

module "cdn" {
  source = "../../modules/cdn"

  providers = {
    aws           = aws
    aws.us_east_1 = aws.us_east_1
  }

  name_prefix  = local.name_prefix
  alb_dns_name = module.platform.alb_dns_name

  origin_verify_header_name  = var.origin_verify_header_name
  origin_verify_header_value = random_password.origin_verify.result

  auth_route_header_name  = var.auth_route_header_name
  auth_route_header_value = var.auth_route_header_value

  waf_rate_limit = var.waf_rate_limit
  price_class    = var.price_class

  tags = local.common_tags
}

module "service_api" {
  source = "../../modules/service"

  name_prefix       = local.name_prefix
  service_name      = "api"
  vpc_id            = module.network.vpc_id
  public_subnet_ids = module.network.public_subnet_ids
  ecs_sg_id         = module.network.ecs_sg_id
  ecs_cluster_id    = module.platform.ecs_cluster_id
  listener_arn      = module.platform.listener_arn
  listener_priority = 20

  # Preserve the exact resource names api's ForceNew resources had under the
  # pre-SPEC-004 modules/app (e.g. "<name_prefix>-tg", not the module's
  # default "<name_prefix>-api-tg"), so moved.tf's cross-module `moved`
  # blocks resolve to a plain move/no-op instead of a replace (see
  # modules/service's target_group_name/task_execution_role_name/
  # task_role_name/secrets_policy_name variables and README "モジュール構成の
  # 変遷"). auth is a new instance and keeps the module's default
  # "<name_prefix>-auth-*" names below.
  target_group_name        = "${local.name_prefix}-tg"
  task_execution_role_name = "${local.name_prefix}-ecs-task-execution"
  task_role_name           = "${local.name_prefix}-ecs-task"
  secrets_policy_name      = "${local.name_prefix}-secrets-read"

  container_image = var.container_image
  container_port  = var.container_port
  task_cpu        = var.task_cpu
  task_memory     = var.task_memory
  desired_count   = var.desired_count

  use_fargate_spot    = var.use_fargate_spot
  fargate_base        = var.fargate_base
  fargate_weight      = var.fargate_weight
  fargate_spot_weight = var.fargate_spot_weight

  route_conditions = [
    { header_name = var.origin_verify_header_name, values = [random_password.origin_verify.result] },
  ]

  # SPEC-005 R6 discrete DB_* contract (app/api's infra/postgres/db.go
  # ConfigFromEnv): host/port/name/sslmode are plain env. DB_NAME="api" is
  # this stack's own dedicated database on the shared RDS instance (SPEC-005
  # RF.1.1 database-per-service; DB_SCHEMA/search_path are no longer used).
  # See the database-bootstrap note on module.service_api's
  # migration_environment below for how that database comes to exist before
  # api's app container ever connects to it.
  environment = [
    { name = "PORT", value = tostring(var.container_port) },
    { name = "DB_HOST", value = module.db.db_endpoint },
    { name = "DB_PORT", value = tostring(module.db.db_port) },
    { name = "DB_NAME", value = "api" },
    { name = "DB_SSLMODE", value = var.db_sslmode },
  ]

  # ISSUE-016 R-c: the app container connects as the least-privilege scoped
  # role api_app, NOT the RDS master user -- so a leaked runtime credential
  # cannot reach the auth database or run DDL against its own. ":username::"
  # / ":password::" select the individual JSON keys of module.db's dedicated
  # api_app secret (ECS's `secret-arn:json-key::` valueFrom syntax), matching
  # infra/postgres/db.go's separate DB_USER/DB_PASSWORD fields. The api_app
  # role itself is not created by Terraform; it's provisioned/synced by the
  # migrate init container below (see migration_secrets' APP_DB_USER/
  # APP_DB_PASSWORD and modules/db/README.md "最小権限ランタイムロール").
  secrets = [
    { name = "DB_USER", valueFrom = "${module.db.api_app_secret_arn}:username::" },
    { name = "DB_PASSWORD", valueFrom = "${module.db.api_app_secret_arn}:password::" },
  ]

  secret_read_arns = [module.db.master_user_secret_arn, module.db.api_app_secret_arn]

  # SPEC-005 R5 migration init container: runs before the app container
  # (modules/service/ecs.tf's dependsOn/condition=SUCCESS wiring). Both api
  # and auth run the same shared app/migrator image (aws_ecr_repository.migrator
  # below), distinguished only by migration_command's "-target api" -- the
  # migrator connects to DB_MAINTENANCE_NAME first to CREATE DATABASE "api"
  # if it doesn't exist yet (RF.1.2 / RF.6.1 RF-a), then reconnects to
  # DB_NAME="api" itself to run goose up against app/api/db/migrations
  # (baked into the shared image). DB_MAINTENANCE_NAME defaults to the
  # "postgres" database that every PostgreSQL server (including this RDS
  # instance) has out of the box; override to module.db.db_name ("app", this
  # RDS instance's own bootstrap database) if "postgres" is ever
  # inaccessible. See modules/service/README.md "マイグレーション init
  # コンテナ" and modules/db/README.md for the full rationale, and
  # docs/plans/SPEC-005-plan.md RF.6 for the concurrency caveat and the
  # deferred image-push wiring -- the shared migrator image referenced here
  # is NOT yet pushed anywhere as of this plan; see this file's README
  # before applying.
  migration_environment = [
    { name = "DB_HOST", value = module.db.db_endpoint },
    { name = "DB_PORT", value = tostring(module.db.db_port) },
    { name = "DB_NAME", value = "api" },
    { name = "DB_SSLMODE", value = var.db_sslmode },
    { name = "DB_MAINTENANCE_NAME", value = "postgres" },
  ]

  # The migrate init container still connects as the RDS master user
  # (DB_USER/DB_PASSWORD, CREATE DATABASE + CREATE ROLE/GRANT need
  # CREATEROLE-equivalent privileges the scoped api_app role must not have).
  # APP_DB_USER/APP_DB_PASSWORD carry api_app's own scoped credentials (same
  # secret as this service's `secrets` above) so app/migrator can provision
  # the role and sync its password before the app container ever starts
  # (ISSUE-016 R-c; see docs/plans/ISSUE-016-plan.md §1.3 and
  # modules/db/README.md "最小権限ランタイムロール").
  migration_secrets = [
    { name = "DB_USER", valueFrom = "${module.db.master_user_secret_arn}:username::" },
    { name = "DB_PASSWORD", valueFrom = "${module.db.master_user_secret_arn}:password::" },
    { name = "APP_DB_USER", valueFrom = "${module.db.api_app_secret_arn}:username::" },
    { name = "APP_DB_PASSWORD", valueFrom = "${module.db.api_app_secret_arn}:password::" },
  ]

  migration_image   = "${aws_ecr_repository.migrator.repository_url}:latest"
  migration_command = ["-target", "api"]

  health_check_path  = var.health_check_path
  log_retention_days = var.log_retention_days

  tags = local.common_tags
}

module "service_auth" {
  source = "../../modules/service"

  name_prefix       = local.name_prefix
  service_name      = "auth"
  vpc_id            = module.network.vpc_id
  public_subnet_ids = module.network.public_subnet_ids
  ecs_sg_id         = module.network.ecs_sg_id
  ecs_cluster_id    = module.platform.ecs_cluster_id
  listener_arn      = module.platform.listener_arn
  listener_priority = 10

  container_image = var.auth_container_image
  container_port  = var.auth_container_port
  task_cpu        = var.auth_task_cpu
  task_memory     = var.auth_task_memory
  desired_count   = var.auth_desired_count

  use_fargate_spot    = var.auth_use_fargate_spot
  fargate_base        = var.auth_fargate_base
  fargate_weight      = var.auth_fargate_weight
  fargate_spot_weight = var.auth_fargate_spot_weight

  route_conditions = [
    { header_name = var.origin_verify_header_name, values = [random_password.origin_verify.result] },
    { header_name = var.auth_route_header_name, values = [var.auth_route_header_value] },
  ]

  # ISSUER carries the /auth prefix that CloudFront's alb-auth behavior
  # strips before forwarding to this service (R5 strip method, see
  # modules/cdn). app/auth is unmodified: it derives every absolute OIDC URL
  # (authorize/token/userinfo/jwks) by concatenating this issuer string, so
  # each resolves back to a path actually reachable through the /auth/*
  # behavior.
  # SPEC-005 R6 discrete DB_* contract (app/auth's infra/postgres/db.go
  # ConfigFromEnv), on the same shared RDS instance as api but its own
  # dedicated database DB_NAME="auth" (SPEC-005 RF.1.1 database-per-service;
  # api and auth no longer share a database or a search_path).
  environment = [
    { name = "PORT", value = tostring(var.auth_container_port) },
    { name = "ISSUER", value = "http://${module.cdn.cloudfront_domain_name}/auth" },
    { name = "DB_HOST", value = module.db.db_endpoint },
    { name = "DB_PORT", value = tostring(module.db.db_port) },
    { name = "DB_NAME", value = "auth" },
    { name = "DB_SSLMODE", value = var.db_sslmode },
  ]

  # ISSUE-016 R-c: same as module.service_api's `secrets` above, but scoped
  # to auth_app (module.db.auth_app_secret_arn) instead of api_app -- auth's
  # runtime credential cannot reach the api database or run DDL against its
  # own. See that block's comment for the ECS `secret-arn:json-key::` syntax.
  secrets = [
    { name = "DB_USER", valueFrom = "${module.db.auth_app_secret_arn}:username::" },
    { name = "DB_PASSWORD", valueFrom = "${module.db.auth_app_secret_arn}:password::" },
  ]

  secret_read_arns = [module.db.master_user_secret_arn, module.db.auth_app_secret_arn]

  # SPEC-005 R5 migration init container; see module.service_api's
  # migration_environment comment above for the full rationale (database
  # bootstrap via app/migrator, concurrency caveat, deferred image push).
  # auth's migrate container runs the same shared app/migrator image at
  # "-target auth", which only ever creates/migrates the "auth" database, so
  # it cannot race with api's migrate container even though both connect to
  # the same RDS instance and the same DB_MAINTENANCE_NAME bootstrap
  # database.
  migration_environment = [
    { name = "DB_HOST", value = module.db.db_endpoint },
    { name = "DB_PORT", value = tostring(module.db.db_port) },
    { name = "DB_NAME", value = "auth" },
    { name = "DB_SSLMODE", value = var.db_sslmode },
    { name = "DB_MAINTENANCE_NAME", value = "postgres" },
  ]

  # ISSUE-016 R-c: master user for CREATE DATABASE/ROLE/GRANT, plus
  # APP_DB_USER/APP_DB_PASSWORD (auth_app's own scoped credentials) so
  # app/migrator can provision/sync the auth_app role. See module.service_api's
  # migration_secrets comment above for the full rationale.
  migration_secrets = [
    { name = "DB_USER", valueFrom = "${module.db.master_user_secret_arn}:username::" },
    { name = "DB_PASSWORD", valueFrom = "${module.db.master_user_secret_arn}:password::" },
    { name = "APP_DB_USER", valueFrom = "${module.db.auth_app_secret_arn}:username::" },
    { name = "APP_DB_PASSWORD", valueFrom = "${module.db.auth_app_secret_arn}:password::" },
  ]

  migration_image   = "${aws_ecr_repository.migrator.repository_url}:latest"
  migration_command = ["-target", "auth"]

  health_check_path  = var.auth_health_check_path
  log_retention_days = var.log_retention_days

  tags = local.common_tags
}
