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

  environment = [
    { name = "PORT", value = tostring(var.container_port) },
    { name = "DB_HOST", value = module.db.db_endpoint },
    { name = "DB_PORT", value = tostring(module.db.db_port) },
    { name = "DB_NAME", value = module.db.db_name },
  ]

  secrets = [
    { name = "DB_CREDENTIALS", valueFrom = module.db.master_user_secret_arn },
  ]

  secret_read_arns = [module.db.master_user_secret_arn]

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
  environment = [
    { name = "PORT", value = tostring(var.auth_container_port) },
    { name = "ISSUER", value = "http://${module.cdn.cloudfront_domain_name}/auth" },
  ]

  secrets          = []
  secret_read_arns = []

  health_check_path  = var.auth_health_check_path
  log_retention_days = var.log_retention_days

  tags = local.common_tags
}
