variable "name_prefix" {
  type        = string
  description = "Prefix applied to all resource names/tags created by this module (e.g. \"cc-orchestrator-dev\")."
}

variable "vpc_cidr" {
  type        = string
  description = "CIDR block for the VPC."
}

variable "azs" {
  type        = list(string)
  description = "Availability zones to spread public/private subnets across. Must have the same length as public_subnet_cidrs and private_subnet_cidrs."
}

variable "public_subnet_cidrs" {
  type        = list(string)
  description = "CIDR blocks for public subnets, one per entry in var.azs (paired by index)."
}

variable "private_subnet_cidrs" {
  type        = list(string)
  description = "CIDR blocks for private subnets, one per entry in var.azs (paired by index)."
}

variable "container_port" {
  type        = number
  description = "TCP port the ECS task listens on. Used to scope the ALB->ECS security group rule."
  default     = 8080
}

variable "db_port" {
  type        = number
  description = "TCP port PostgreSQL listens on. Used to scope the ECS->RDS security group rule."
  default     = 5432
}

variable "tags" {
  type        = map(string)
  description = "Common tags applied to all resources created by this module."
  default     = {}
}
