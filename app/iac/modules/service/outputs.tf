output "ecr_repository_url" {
  description = "Repository URL of the ECR repository created for this service's image."
  value       = aws_ecr_repository.this.repository_url
}

output "target_group_arn" {
  description = "ARN of this service's ALB target group."
  value       = aws_lb_target_group.this.arn
}

output "ecs_service_name" {
  description = "Name of the ECS service."
  value       = aws_ecs_service.this.name
}

output "log_group_name" {
  description = "Name of the CloudWatch Logs group used by this service's ECS task."
  value       = aws_cloudwatch_log_group.this.name
}
