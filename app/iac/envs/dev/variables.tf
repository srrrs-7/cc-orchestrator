variable "region" {
  type        = string
  description = "Primary AWS region for all resources except the CDN module's WAFv2 Web ACL (which requires us-east-1)."
  default     = "ap-northeast-1"
}

variable "project" {
  type        = string
  description = "Project name used to derive resource name prefixes and the Project tag."
  default     = "cc-orchestrator"
}

variable "environment" {
  type        = string
  description = "Environment name used to derive resource name prefixes and the Environment tag."
  default     = "dev"
}

# --- network -----------------------------------------------------------------

variable "vpc_cidr" {
  type        = string
  description = "CIDR block for the VPC."
  default     = "10.0.0.0/16"
}

variable "azs" {
  type        = list(string)
  description = "Availability zones to spread subnets across."
  default     = ["ap-northeast-1a", "ap-northeast-1c"]
}

variable "public_subnet_cidrs" {
  type        = list(string)
  description = "CIDR blocks for public subnets, one per entry in var.azs."
  default     = ["10.0.0.0/24", "10.0.1.0/24"]
}

variable "private_subnet_cidrs" {
  type        = list(string)
  description = "CIDR blocks for private subnets, one per entry in var.azs."
  default     = ["10.0.10.0/24", "10.0.11.0/24"]
}

variable "container_port" {
  type        = number
  description = "TCP port the ECS task/container listens on (app/api's defaultPort)."
  default     = 8080
}

variable "db_port" {
  type        = number
  description = "TCP port PostgreSQL listens on."
  default     = 5432
}

# --- db ------------------------------------------------------------------------

variable "db_instance_class" {
  type        = string
  description = "RDS instance class."
  default     = "db.t4g.micro"
}

variable "db_allocated_storage" {
  type        = number
  description = "Allocated storage for the RDS instance, in GiB."
  default     = 20
}

variable "db_engine_version" {
  type        = string
  description = "PostgreSQL engine version (major.minor)."
  default     = "16.4"
}

variable "db_name" {
  type        = string
  description = "Initial database name."
  default     = "app"
}

variable "db_master_username" {
  type        = string
  description = "RDS master username (the password itself is managed by manage_master_user_password, not this variable)."
  default     = "app_admin"

  # ISSUE-016 review-security Major-2: mutual-distinctness of this variable
  # vs. db_api_app_role_name/db_auth_app_role_name is enforced by
  # db_auth_app_role_name's validation below, in one place, to avoid a
  # dependency cycle (Terraform 1.9+ cross-variable validation only allows a
  # one-directional reference graph; each of the three variables validating
  # against the other two forms a cycle -- see that variable's comment for
  # the full rationale).
}

variable "db_multi_az" {
  type        = bool
  description = "Whether to enable RDS Multi-AZ. Defaults to false (single-AZ) to minimize dev cost."
  default     = false
}

variable "db_deletion_protection" {
  type        = bool
  description = "Whether to enable RDS deletion protection."
  default     = false
}

variable "db_skip_final_snapshot" {
  type        = bool
  description = "Whether to skip the final RDS snapshot on destroy."
  default     = true
}

variable "db_backup_retention_period" {
  type        = number
  description = "Number of days to retain automated RDS backups."
  default     = 1
}

variable "db_sslmode" {
  type        = string
  description = "libpq sslmode used by both api and auth to connect to RDS (SPEC-005 R6, injected as the DB_SSLMODE env var). Defaults to \"require\", which encrypts the connection (RDS PostgreSQL endpoints always offer TLS) without verifying the server certificate against a CA bundle; \"verify-full\" would additionally need the RDS CA bundle distributed into both app images, which is out of scope here."
  default     = "require"
}

variable "db_api_app_role_name" {
  type        = string
  description = "PostgreSQL role name for app/api's least-privilege runtime connection (ISSUE-016 R-c). Only the role's credentials are provisioned by module.db (random_password + Secrets Manager); the role itself is created by app/migrator's migrate init container."
  default     = "api_app"

  # ISSUE-016 review-security Major-2: mutual-distinctness enforced by
  # db_auth_app_role_name's validation below (see that variable's comment).
}

variable "db_auth_app_role_name" {
  type        = string
  description = "PostgreSQL role name for app/auth's least-privilege runtime connection (ISSUE-016 R-c). Same caveat as db_api_app_role_name."
  default     = "auth_app"

  # ISSUE-016 review-security Major-2: master username (db_master_username)
  # and the two scoped runtime role names (db_api_app_role_name, this
  # variable) must be pairwise distinct. If any two collided, app/migrator's
  # idempotent role-credential sync (`ALTER ROLE <name> PASSWORD ...`, run by
  # the migrate init container to keep each scoped role's password in sync
  # with its Secrets Manager secret) would resolve the colliding name to a
  # single PostgreSQL role and overwrite that role's password with the wrong
  # value -- most dangerously, overwriting the RDS *master* password with a
  # scoped app role's randomly generated one, silently locking out master
  # access. Fail closed here at plan/apply time instead of at migrate-init
  # runtime.
  #
  # All three pairwise comparisons are checked from this single variable's
  # validation (not from each of the three) because Terraform 1.9+
  # cross-variable validation forms a dependency graph from the reference,
  # and three variables each validating against the other two creates a
  # cycle ("Cycle: var.db_auth_app_role_name (validation), ... (validation)"
  # at `terraform validate`). A one-directional reference graph (only this
  # variable references the other two; they don't reference back) avoids
  # that cycle while still covering all three pairs.
  #
  # Duplicated at the module.db boundary (modules/db/variables.tf's
  # auth_app_role_name) so the guard holds even if module.db is reused from
  # a future env root module without this file.
  validation {
    condition = (
      var.db_master_username != var.db_api_app_role_name &&
      var.db_master_username != var.db_auth_app_role_name &&
      var.db_api_app_role_name != var.db_auth_app_role_name
    )
    error_message = "db_master_username, db_api_app_role_name and db_auth_app_role_name must all be pairwise distinct (ISSUE-016 Major-2): a collision would make app/migrator's role-credential sync overwrite the colliding role's password (most dangerously, the RDS master password) with the wrong value."
  }
}

# --- app -----------------------------------------------------------------------

variable "container_image" {
  type        = string
  description = "Container image URI (repository:tag) for the ECS task. Leave empty to default to this environment's own ECR repository at \":latest\"."
  default     = ""
}

variable "task_cpu" {
  type        = number
  description = "Fargate task vCPU units."
  default     = 256
}

variable "task_memory" {
  type        = number
  description = "Fargate task memory in MiB."
  default     = 512
}

variable "desired_count" {
  type        = number
  description = "Desired number of running ECS tasks."
  default     = 1
}

variable "use_fargate_spot" {
  type        = bool
  description = "Whether to mix FARGATE_SPOT capacity into the service (R5)."
  default     = true
}

variable "fargate_base" {
  type        = number
  description = "Minimum number of tasks kept on on-demand FARGATE capacity when use_fargate_spot is true."
  default     = 0
}

variable "fargate_weight" {
  type        = number
  description = "Relative weight of on-demand FARGATE capacity when use_fargate_spot is true."
  default     = 0
}

variable "fargate_spot_weight" {
  type        = number
  description = "Relative weight of FARGATE_SPOT capacity when use_fargate_spot is true."
  default     = 1
}

variable "origin_verify_header_name" {
  type        = string
  description = "HTTP header name used for the CloudFront->ALB origin-verify custom header, shared by both the api and auth ALB origins (R3)."
  default     = "X-Origin-Verify"
}

variable "health_check_path" {
  type        = string
  description = "api ALB target group health check path. Defaults to \"/health\" (unauthenticated liveness probe; task endpoints require Bearer JWT since SPEC-015)."
  default     = "/health"
}

variable "log_retention_days" {
  type        = number
  description = "CloudWatch Logs retention period in days for the ECS task logs (shared by api and auth)."
  default     = 14
}

# --- auth (app/auth, 2nd modules/service instance) ------------------------------

variable "auth_container_image" {
  type        = string
  description = "Container image URI (repository:tag) for the auth ECS task. Leave empty to default to this environment's own auth ECR repository at \":latest\"."
  default     = ""
}

variable "auth_container_port" {
  type        = number
  description = "TCP port the auth container listens on (app/auth's defaultPort)."
  default     = 8080
}

variable "auth_task_cpu" {
  type        = number
  description = "Fargate task vCPU units for the auth service."
  default     = 256
}

variable "auth_task_memory" {
  type        = number
  description = "Fargate task memory in MiB for the auth service."
  default     = 512
}

variable "auth_desired_count" {
  type        = number
  description = "Desired number of running auth ECS tasks. Defaults to 1: app/auth generates a new RSA signing key per process, so multiple concurrent tasks would have mismatched JWKS/kid and break token/userinfo verification across tasks (see modules/service/README.md)."
  default     = 1
}

variable "auth_use_fargate_spot" {
  type        = bool
  description = "Whether to mix FARGATE_SPOT capacity into the auth service's capacity provider strategy."
  default     = true
}

variable "auth_fargate_base" {
  type        = number
  description = "Minimum number of auth tasks kept on on-demand FARGATE capacity when auth_use_fargate_spot is true."
  default     = 0
}

variable "auth_fargate_weight" {
  type        = number
  description = "Relative weight of on-demand FARGATE capacity for the auth service when auth_use_fargate_spot is true."
  default     = 0
}

variable "auth_fargate_spot_weight" {
  type        = number
  description = "Relative weight of FARGATE_SPOT capacity for the auth service when auth_use_fargate_spot is true."
  default     = 1
}

variable "auth_health_check_path" {
  type        = string
  description = "auth ALB target group health check path (OIDC discovery endpoint, matcher = 200)."
  default     = "/.well-known/openid-configuration"
}

variable "auth_route_header_name" {
  type        = string
  description = "HTTP header name CloudFront injects on the alb-auth origin only (in addition to origin_verify_header_name) so the ALB listener rule routes to the auth target group instead of api's. Passed to both the cdn module (as a custom origin header) and the auth service instance (as a route_conditions header), which must stay in sync."
  default     = "X-Target-Service"
}

variable "auth_route_header_value" {
  type        = string
  description = "Expected value of auth_route_header_name. Not a secret (routing discriminator only, see modules/cdn/README.md)."
  default     = "auth"
}

# --- cdn -----------------------------------------------------------------------

variable "waf_rate_limit" {
  type        = number
  description = "Maximum requests allowed per IP per 5-minute window before WAF blocks it."
  default     = 2000
}

variable "price_class" {
  type        = string
  description = "CloudFront distribution price class."
  default     = "PriceClass_100"
}
