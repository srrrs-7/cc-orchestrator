variable "name_prefix" {
  type        = string
  description = "Prefix applied to all resource names/tags created by this module."
}

variable "alb_dns_name" {
  type        = string
  description = "ALB DNS name used as the CloudFront custom origin (from the app module)."
}

variable "origin_verify_header_name" {
  type        = string
  description = "HTTP header name injected as a CloudFront custom origin header; must match the ALB listener rule's expected header name in the app module (R3)."
}

variable "origin_verify_header_value" {
  type        = string
  description = "Expected value of the origin-verify header. Generated once in envs/dev via random_password and shared with the app module; never written to tfvars in plain text."
  sensitive   = true
}

variable "waf_rate_limit" {
  type        = number
  description = "Maximum number of requests allowed from a single IP within a 5-minute window before WAF blocks it (rate_based_statement limit). Must be >= 100 per the WAFv2 API."
  default     = 2000

  validation {
    condition     = var.waf_rate_limit >= 100
    error_message = "waf_rate_limit must be >= 100 (WAFv2 rate_based_statement minimum)."
  }
}

variable "price_class" {
  type        = string
  description = "CloudFront distribution price class."
  default     = "PriceClass_100"

  validation {
    condition     = contains(["PriceClass_100", "PriceClass_200", "PriceClass_All"], var.price_class)
    error_message = "price_class must be one of PriceClass_100, PriceClass_200, PriceClass_All."
  }
}

variable "tags" {
  type        = map(string)
  description = "Common tags applied to all resources created by this module."
  default     = {}
}
