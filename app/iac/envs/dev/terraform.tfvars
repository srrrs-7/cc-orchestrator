# dev environment values. No secrets: DB master password is managed by RDS
# via Secrets Manager (manage_master_user_password); the CloudFront<->ALB
# origin-verify header value is generated at plan/apply time by
# random_password and is never written here.

region      = "ap-northeast-1"
project     = "cc-orchestrator"
environment = "dev"

vpc_cidr             = "10.0.0.0/16"
azs                  = ["ap-northeast-1a", "ap-northeast-1c"]
public_subnet_cidrs  = ["10.0.0.0/24", "10.0.1.0/24"]
private_subnet_cidrs = ["10.0.10.0/24", "10.0.11.0/24"]

container_port = 8080
db_port        = 5432

db_instance_class          = "db.t4g.micro"
db_allocated_storage       = 20
db_engine_version          = "16.4"
db_name                    = "app"
db_master_username         = "app_admin"
db_multi_az                = false
db_deletion_protection     = false
db_skip_final_snapshot     = true
db_backup_retention_period = 1

container_image = ""
task_cpu        = 256
task_memory     = 512
desired_count   = 1

use_fargate_spot    = true
fargate_base        = 0
fargate_weight      = 0
fargate_spot_weight = 1

origin_verify_header_name = "X-Origin-Verify"
health_check_path         = "/tasks"
log_retention_days        = 14

# auth (app/auth). desired_count is fixed at 1: app/auth generates a new RSA
# signing key per process, so multiple concurrent tasks would break
# token/userinfo verification across tasks (see modules/service/README.md).
auth_container_image     = ""
auth_container_port      = 8080
auth_task_cpu            = 256
auth_task_memory         = 512
auth_desired_count       = 1
auth_use_fargate_spot    = true
auth_fargate_base        = 0
auth_fargate_weight      = 0
auth_fargate_spot_weight = 1
auth_health_check_path   = "/.well-known/openid-configuration"
auth_route_header_name   = "X-Target-Service"
auth_route_header_value  = "auth"

waf_rate_limit = 2000
price_class    = "PriceClass_100"
