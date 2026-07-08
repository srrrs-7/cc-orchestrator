output "cloudfront_domain_name" {
  description = "CloudFront distribution's default domain name; the public entry point of the API (R1)."
  value       = module.cdn.cloudfront_domain_name
}

output "alb_dns_name" {
  description = "ALB DNS name (CloudFront origin; not intended for direct client access, see modules/network README)."
  value       = module.app.alb_dns_name
}

output "ecr_repository_url" {
  description = "ECR repository URL to push the app/api container image to."
  value       = module.app.ecr_repository_url
}

output "rds_endpoint" {
  description = "RDS instance hostname."
  value       = module.db.db_endpoint
}

output "ecs_cluster_name" {
  description = "Name of the ECS cluster."
  value       = module.app.ecs_cluster_name
}

output "ecs_service_name" {
  description = "Name of the ECS service."
  value       = module.app.ecs_service_name
}

output "web_acl_arn" {
  description = "ARN of the WAFv2 Web ACL protecting the CloudFront distribution."
  value       = module.cdn.web_acl_arn
}
