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
  description = "HTTP header name used for the CloudFront->ALB origin-verify custom header (R3)."
  default     = "X-Origin-Verify"
}

variable "health_check_path" {
  type        = string
  description = "ALB target group health check path. Defaults to \"/tasks\" because app/api has no dedicated health endpoint yet (see ISSUE-001)."
  default     = "/tasks"
}

variable "log_retention_days" {
  type        = number
  description = "CloudWatch Logs retention period in days for the ECS task logs."
  default     = 14
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
