# ECS cluster, Fargate task definition (ARM64) and service.
#
# Cost/design notes:
#   - runtime_platform.cpu_architecture = ARM64 (Graviton) is cheaper per
#     vCPU/GB than x86_64 Fargate (R5). Any image pushed to the ECR
#     repository must be built for linux/arm64.
#   - capacity_provider_strategy mixes FARGATE (on-demand baseline) and
#     FARGATE_SPOT so that dev workloads can run mostly/entirely on Spot
#     capacity for further savings; see variables.tf and README for the
#     weight/base defaults.
#   - ECS tasks run in public subnets with assign_public_ip = true instead of
#     a NAT Gateway (R6); egress is restricted to the security group defined
#     in the network module.

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

resource "aws_ecs_task_definition" "this" {
  family                   = "${var.name_prefix}-api"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = tostring(var.task_cpu)
  memory                   = tostring(var.task_memory)
  execution_role_arn       = aws_iam_role.task_execution.arn
  task_role_arn            = aws_iam_role.task.arn

  runtime_platform {
    cpu_architecture        = "ARM64"
    operating_system_family = "LINUX"
  }

  container_definitions = jsonencode([
    {
      name      = "api"
      image     = local.container_image
      essential = true

      portMappings = [
        {
          containerPort = var.container_port
          protocol      = "tcp"
        }
      ]

      environment = [
        { name = "PORT", value = tostring(var.container_port) },
        { name = "DB_HOST", value = var.db_endpoint },
        { name = "DB_PORT", value = tostring(var.db_port) },
        { name = "DB_NAME", value = var.db_name },
      ]

      # Injects the Secrets Manager JSON secret (username/password) as the
      # DB_CREDENTIALS env var at container start; the container never
      # receives a plaintext credential via Terraform state or tfvars.
      secrets = [
        {
          name      = "DB_CREDENTIALS"
          valueFrom = var.db_secret_arn
        }
      ]

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.this.name
          "awslogs-region"        = data.aws_region.current.name
          "awslogs-stream-prefix" = "api"
        }
      }
    }
  ])

  tags = merge(var.tags, { Name = "${var.name_prefix}-api" })
}

resource "aws_ecs_service" "this" {
  name            = "${var.name_prefix}-api"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.this.arn
  desired_count   = var.desired_count

  # Cost-optimized capacity mix: when Fargate Spot is enabled, only
  # var.fargate_base tasks run on-demand and the rest are scheduled on Spot
  # capacity (default: fully Spot). When disabled, the service runs entirely
  # on on-demand Fargate.
  dynamic "capacity_provider_strategy" {
    for_each = var.use_fargate_spot ? [
      { capacity_provider = "FARGATE", weight = var.fargate_weight, base = var.fargate_base },
      { capacity_provider = "FARGATE_SPOT", weight = var.fargate_spot_weight, base = 0 },
      ] : [
      { capacity_provider = "FARGATE", weight = 1, base = 0 },
    ]

    content {
      capacity_provider = capacity_provider_strategy.value.capacity_provider
      weight            = capacity_provider_strategy.value.weight
      base              = capacity_provider_strategy.value.base
    }
  }

  network_configuration {
    subnets          = var.public_subnet_ids
    security_groups  = [var.ecs_sg_id]
    assign_public_ip = true
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.app.arn
    container_name   = "api"
    container_port   = var.container_port
  }

  # Ensure the listener rule (and therefore the listener/target group) exists
  # before the service tries to register tasks with the target group.
  depends_on = [aws_lb_listener_rule.verified_origin]

  tags = merge(var.tags, { Name = "${var.name_prefix}-api" })
}
