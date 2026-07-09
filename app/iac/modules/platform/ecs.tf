# Shared ECS cluster and capacity providers. Individual services' task
# definitions and ECS services are defined by modules/service, which
# registers against this cluster via var.ecs_cluster_id (this module's
# ecs_cluster_id output).

resource "aws_ecs_cluster" "this" {
  name = "${var.name_prefix}-cluster"

  setting {
    name  = "containerInsights"
    value = "disabled"
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-cluster" })
}

resource "aws_ecs_cluster_capacity_providers" "this" {
  cluster_name       = aws_ecs_cluster.this.name
  capacity_providers = ["FARGATE", "FARGATE_SPOT"]
}
