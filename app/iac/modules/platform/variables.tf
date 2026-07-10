variable "name_prefix" {
  type        = string
  description = "Prefix applied to all resource names/tags created by this module."
}

variable "vpc_id" {
  type        = string
  description = "VPC ID the platform resources are created in."
}

variable "public_subnet_ids" {
  type        = list(string)
  description = "Public subnet IDs the ALB is placed in (no NAT Gateway is used, see network module README)."
}

variable "alb_sg_id" {
  type        = string
  description = "Security group ID to attach to the ALB."
}

variable "tags" {
  type        = map(string)
  description = "Common tags applied to all resources created by this module."
  default     = {}
}
