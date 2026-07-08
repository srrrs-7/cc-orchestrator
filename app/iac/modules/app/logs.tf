# CloudWatch Logs group for the ECS task's awslogs driver.

resource "aws_cloudwatch_log_group" "this" {
  name              = "/ecs/${var.name_prefix}-api"
  retention_in_days = var.log_retention_days

  tags = merge(var.tags, { Name = "${var.name_prefix}-api-logs" })
}
