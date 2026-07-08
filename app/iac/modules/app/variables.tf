variable "name_prefix" {
  type        = string
  description = "Prefix applied to all resource names/tags created by this module."
}

variable "vpc_id" {
  type        = string
  description = "VPC ID the ALB/ECS resources are created in."
}

variable "public_subnet_ids" {
  type        = list(string)
  description = "Public subnet IDs used by both the ALB and the ECS service (no NAT Gateway is used, see README)."
}

variable "alb_sg_id" {
  type        = string
  description = "Security group ID to attach to the ALB."
}

variable "ecs_sg_id" {
  type        = string
  description = "Security group ID to attach to the ECS tasks."
}

variable "container_image" {
  type        = string
  description = "Container image URI (repository:tag) for the ECS task. Leave empty (default) to use this module's own ECR repository at the \":latest\" tag; no image is built/pushed by Terraform, so the service will not become healthy until an image is pushed."
  default     = ""
}

variable "container_port" {
  type        = number
  description = "TCP port the container listens on (app/api's defaultPort)."
  default     = 8080
}

variable "task_cpu" {
  type        = number
  description = "Fargate task vCPU units (e.g. 256 = 0.25 vCPU)."
  default     = 256
}

variable "task_memory" {
  type        = number
  description = "Fargate task memory in MiB (e.g. 512)."
  default     = 512
}

variable "desired_count" {
  type        = number
  description = "Desired number of running ECS tasks."
  default     = 1
}

variable "use_fargate_spot" {
  type        = bool
  description = "Whether to mix FARGATE_SPOT capacity into the service's capacity provider strategy. Recommended for dev to reduce cost (R5)."
  default     = true
}

variable "fargate_base" {
  type        = number
  description = "Minimum number of tasks to keep on on-demand FARGATE capacity when use_fargate_spot is true (0 = fully Spot)."
  default     = 0
}

variable "fargate_weight" {
  type        = number
  description = "Relative weight of on-demand FARGATE capacity (beyond fargate_base) when use_fargate_spot is true."
  default     = 0
}

variable "fargate_spot_weight" {
  type        = number
  description = "Relative weight of FARGATE_SPOT capacity when use_fargate_spot is true."
  default     = 1
}

variable "origin_verify_header_name" {
  type        = string
  description = "HTTP header name CloudFront injects as a custom origin header; the ALB listener rule only forwards requests carrying this header with the expected value (R3)."
}

variable "origin_verify_header_value" {
  type        = string
  description = "Expected value of the origin-verify header. Generated once in envs/dev via random_password and shared with the cdn module; never written to tfvars in plain text."
  sensitive   = true
}

variable "db_secret_arn" {
  type        = string
  description = "ARN of the Secrets Manager secret holding the RDS master user credentials (from the db module), injected into the task as a \"secrets\" entry."
}

variable "db_endpoint" {
  type        = string
  description = "RDS instance hostname, passed to the task definition as the DB_HOST environment variable."
}

variable "db_port" {
  type        = number
  description = "RDS instance port, passed to the task definition as the DB_PORT environment variable."
  default     = 5432
}

variable "db_name" {
  type        = string
  description = "Database name, passed to the task definition as the DB_NAME environment variable."
}

variable "health_check_path" {
  type        = string
  description = "ALB target group health check path. Defaults to \"/tasks\" because app/api has no dedicated health endpoint yet (see README / ISSUE-001)."
  default     = "/tasks"
}

variable "log_retention_days" {
  type        = number
  description = "CloudWatch Logs retention period in days for the ECS task logs."
  default     = 14
}

variable "tags" {
  type        = map(string)
  description = "Common tags applied to all resources created by this module."
  default     = {}
}
