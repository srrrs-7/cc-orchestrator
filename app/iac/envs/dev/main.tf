locals {
  name_prefix = "${var.project}-${var.environment}"

  common_tags = {
    Project     = var.project
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}

# Single generation point for the CloudFront <-> ALB origin-verify secret
# shared between the app and cdn modules (see modules/app and modules/cdn
# README for the two-layer defense this implements, R3). Alphanumeric-only
# to avoid characters that are invalid in HTTP header values.
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

module "app" {
  source = "../../modules/app"

  name_prefix       = local.name_prefix
  vpc_id            = module.network.vpc_id
  public_subnet_ids = module.network.public_subnet_ids
  alb_sg_id         = module.network.alb_sg_id
  ecs_sg_id         = module.network.ecs_sg_id

  container_image = var.container_image
  container_port  = var.container_port
  task_cpu        = var.task_cpu
  task_memory     = var.task_memory
  desired_count   = var.desired_count

  use_fargate_spot    = var.use_fargate_spot
  fargate_base        = var.fargate_base
  fargate_weight      = var.fargate_weight
  fargate_spot_weight = var.fargate_spot_weight

  origin_verify_header_name  = var.origin_verify_header_name
  origin_verify_header_value = random_password.origin_verify.result

  db_secret_arn = module.db.master_user_secret_arn
  db_endpoint   = module.db.db_endpoint
  db_port       = module.db.db_port
  db_name       = module.db.db_name

  health_check_path  = var.health_check_path
  log_retention_days = var.log_retention_days

  tags = local.common_tags
}

module "cdn" {
  source = "../../modules/cdn"

  providers = {
    aws           = aws
    aws.us_east_1 = aws.us_east_1
  }

  name_prefix  = local.name_prefix
  alb_dns_name = module.app.alb_dns_name

  origin_verify_header_name  = var.origin_verify_header_name
  origin_verify_header_value = random_password.origin_verify.result

  waf_rate_limit = var.waf_rate_limit
  price_class    = var.price_class

  tags = local.common_tags
}
