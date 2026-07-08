variable "name_prefix" {
  type        = string
  description = "Prefix applied to all resource names/tags created by this module."
}

variable "private_subnet_ids" {
  type        = list(string)
  description = "Private subnet IDs for the DB subnet group (RDS is not placed in public subnets)."
}

variable "rds_sg_id" {
  type        = string
  description = "Security group ID to attach to the RDS instance (must allow inbound only from the ECS security group)."
}

variable "instance_class" {
  type        = string
  description = "RDS instance class."
  default     = "db.t4g.micro"
}

variable "allocated_storage" {
  type        = number
  description = "Allocated storage for the RDS instance, in GiB."
  default     = 20
}

variable "engine_version" {
  type        = string
  description = "PostgreSQL engine version (major.minor, e.g. \"16.4\")."
  default     = "16.4"
}

variable "db_name" {
  type        = string
  description = "Initial database name created on the RDS instance."
  default     = "app"
}

variable "master_username" {
  type        = string
  description = "Master username for the RDS instance. Not a secret by itself; the password is managed by RDS/Secrets Manager (manage_master_user_password)."
  default     = "app_admin"
}

variable "multi_az" {
  type        = bool
  description = "Whether to enable Multi-AZ deployment. Defaults to false (single-AZ) to minimize dev cost; see module README for the availability trade-off."
  default     = false
}

variable "deletion_protection" {
  type        = bool
  description = "Whether to enable RDS deletion protection."
  default     = false
}

variable "skip_final_snapshot" {
  type        = bool
  description = "Whether to skip the final snapshot on destroy. Defaults to true for a disposable dev environment."
  default     = true
}

variable "backup_retention_period" {
  type        = number
  description = "Number of days to retain automated backups."
  default     = 1
}

variable "tags" {
  type        = map(string)
  description = "Common tags applied to all resources created by this module."
  default     = {}
}
