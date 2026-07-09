variable "name_prefix" {
  type        = string
  description = "Prefix applied to all resource names/tags created by this module."
}

variable "service_name" {
  type        = string
  description = "Short name identifying this service instance (e.g. \"api\", \"auth\"). Used to derive resource names (target group, ECR repository, log group, task family/container name, IAM role names) so that multiple instances of this module can coexist under the same name_prefix."
}

variable "vpc_id" {
  type        = string
  description = "VPC ID the target group is created in."
}

variable "public_subnet_ids" {
  type        = list(string)
  description = "Public subnet IDs used by the ECS service's tasks (no NAT Gateway is used, see network module README)."
}

variable "ecs_sg_id" {
  type        = string
  description = "Security group ID to attach to the ECS tasks."
}

variable "ecs_cluster_id" {
  type        = string
  description = "ID (ARN) of the shared ECS cluster this service's tasks run on (from modules/platform)."
}

variable "listener_arn" {
  type        = string
  description = "ARN of the shared ALB HTTP listener this service's listener rule attaches to (from modules/platform)."
}

variable "listener_priority" {
  type        = number
  description = "Priority of this service's ALB listener rule. Must be unique across all services sharing the same listener; lower values are evaluated first."
}

variable "container_image" {
  type        = string
  description = "Container image URI (repository:tag) for the ECS task. Leave empty (default) to use this module's own ECR repository at the \":latest\" tag; no image is built/pushed by Terraform, so the service will not become healthy until an image is pushed."
  default     = ""
}

variable "container_port" {
  type        = number
  description = "TCP port the container listens on."
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
  description = "Whether to mix FARGATE_SPOT capacity into the service's capacity provider strategy. Recommended for dev to reduce cost."
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

variable "route_conditions" {
  type = list(object({
    header_name = string
    values      = list(string)
  }))
  description = "ALB listener rule HTTP header conditions for this service. All entries are ANDed (multiple `condition` blocks on the same rule): e.g. api = [{header_name = \"X-Origin-Verify\", values = [<secret>]}], auth = [{header_name = \"X-Origin-Verify\", values = [<secret>]}, {header_name = \"X-Target-Service\", values = [\"auth\"]}]. Needed because CloudFront strips the /api or /auth path prefix before forwarding to the ALB, so paths alone cannot distinguish services at the ALB (see modules/cdn and listener_rule.tf)."
}

variable "environment" {
  type = list(object({
    name  = string
    value = string
  }))
  description = "Plain (non-secret) environment variables injected into the container, in ECS container definition \"environment\" format (e.g. PORT, DB_HOST, ISSUER)."
  default     = []
}

variable "secrets" {
  type = list(object({
    name      = string
    valueFrom = string
  }))
  description = "Secrets Manager/SSM-backed environment variables injected into the container at start, in ECS container definition \"secrets\" format (e.g. DB_CREDENTIALS). Never contains a plaintext value; valueFrom is an ARN resolved by the ECS agent using the task execution role."
  default     = []
  sensitive   = true
}

variable "secret_read_arns" {
  type        = list(string)
  description = "ARNs of Secrets Manager secrets the task execution role must be able to read (secretsmanager:GetSecretValue), matching the valueFrom ARNs referenced in var.secrets. Leave empty (default) if the service has no secrets (e.g. auth)."
  default     = []
}

variable "target_group_name" {
  type        = string
  description = "Override for the ALB target group's `name` (and its Name tag). aws_lb_target_group.name is ForceNew (no rename API), so leave unset (default null) to use \"<name_prefix>-<service_name>-tg\" for a fresh service instance, or pass an existing name to preserve it across a module refactor without a replace (e.g. api reuses the \"<name_prefix>-tg\" name it had under the pre-SPEC-004 modules/app, see envs/dev/main.tf and moved.tf)."
  default     = null
}

variable "task_execution_role_name" {
  type        = string
  description = "Override for the ECS task execution IAM role's `name` (and its Name tag). aws_iam_role.name is ForceNew, so leave unset (default null) to use \"<name_prefix>-<service_name>-ecs-task-execution\", or pass an existing name to preserve it (see target_group_name)."
  default     = null
}

variable "task_role_name" {
  type        = string
  description = "Override for the ECS task IAM role's `name` (and its Name tag). aws_iam_role.name is ForceNew, so leave unset (default null) to use \"<name_prefix>-<service_name>-ecs-task\", or pass an existing name to preserve it (see target_group_name)."
  default     = null
}

variable "secrets_policy_name" {
  type        = string
  description = "Override for the task execution role's inline secrets-read IAM policy `name`. aws_iam_role_policy.name is ForceNew, so leave unset (default null) to use \"<name_prefix>-<service_name>-secrets-read\", or pass an existing name to preserve it (see target_group_name)."
  default     = null
}

variable "health_check_path" {
  type        = string
  description = "ALB target group health check path (e.g. \"/tasks\" for api, \"/.well-known/openid-configuration\" for auth; matcher = 200)."
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
