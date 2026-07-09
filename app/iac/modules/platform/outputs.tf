output "alb_dns_name" {
  description = "ALB default DNS name; used as the CloudFront origin (both alb-api and alb-auth origins point at the same ALB, see cdn module)."
  value       = aws_lb.this.dns_name
}

output "alb_arn" {
  description = "ARN of the ALB."
  value       = aws_lb.this.arn
}

output "listener_arn" {
  description = "ARN of the shared HTTP listener. Each modules/service instance attaches its own listener rule to this ARN."
  value       = aws_lb_listener.http.arn
}

output "ecs_cluster_id" {
  description = "ID (ARN) of the shared ECS cluster. Each modules/service instance's ECS service registers against this cluster."
  value       = aws_ecs_cluster.this.id
}
