output "cloudfront_domain_name" {
  description = "CloudFront distribution's default domain name (*.cloudfront.net)."
  value       = aws_cloudfront_distribution.this.domain_name
}

output "cloudfront_distribution_id" {
  description = "CloudFront distribution ID."
  value       = aws_cloudfront_distribution.this.id
}

output "web_acl_arn" {
  description = "ARN of the WAFv2 Web ACL (CLOUDFRONT scope)."
  value       = aws_wafv2_web_acl.this.arn
}

output "web_bucket_name" {
  description = "Name of the S3 bucket holding the web SPA's static assets. Used by build-push tooling (`aws s3 sync dist s3://<this>`)."
  value       = aws_s3_bucket.web.bucket
}

output "web_bucket_arn" {
  description = "ARN of the S3 bucket holding the web SPA's static assets."
  value       = aws_s3_bucket.web.arn
}
