output "cloudfront_domain_name" {
  description = "CloudFront distribution's default domain name; the public entry point of web/api/auth (R1-R3)."
  value       = module.cdn.cloudfront_domain_name
}

output "cloudfront_distribution_id" {
  description = "CloudFront distribution ID; used by build-push tooling to invalidate the cache after a web deploy."
  value       = module.cdn.cloudfront_distribution_id
}

output "web_url" {
  description = "Public HTTPS URL of the web SPA (CloudFront default domain)."
  value       = "https://${module.cdn.cloudfront_domain_name}"
}

output "web_bucket_name" {
  description = "Name of the S3 bucket to sync the web SPA's built assets to (`aws s3 sync dist s3://<this> --delete`)."
  value       = module.cdn.web_bucket_name
}

output "alb_dns_name" {
  description = "ALB DNS name (CloudFront origin; not intended for direct client access, see modules/network README)."
  value       = module.platform.alb_dns_name
}

output "api_ecr_repository_url" {
  description = "ECR repository URL to push the app/api container image to (linux/arm64, see modules/service README)."
  value       = module.service_api.ecr_repository_url
}

output "auth_ecr_repository_url" {
  description = "ECR repository URL to push the app/auth container image to (linux/arm64, see modules/service README)."
  value       = module.service_auth.ecr_repository_url
}

output "rds_endpoint" {
  description = "RDS instance hostname."
  value       = module.db.db_endpoint
}

output "ecs_cluster_id" {
  description = "ID (ARN) of the shared ECS cluster running both the api and auth services."
  value       = module.platform.ecs_cluster_id
}

output "api_ecs_service_name" {
  description = "Name of the api ECS service."
  value       = module.service_api.ecs_service_name
}

output "auth_ecs_service_name" {
  description = "Name of the auth ECS service."
  value       = module.service_auth.ecs_service_name
}

output "web_acl_arn" {
  description = "ARN of the WAFv2 Web ACL protecting the CloudFront distribution."
  value       = module.cdn.web_acl_arn
}
