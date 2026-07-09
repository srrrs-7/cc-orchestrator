# Fargate task definition (ARM64) and ECS service for this service instance.
# The ECS cluster itself is shared and lives in modules/platform
# (var.ecs_cluster_id).
#
# Cost/design notes:
#   - runtime_platform.cpu_architecture = ARM64 (Graviton) is cheaper per
#     vCPU/GB than x86_64 Fargate. Any image pushed to the ECR repository
#     must be built for linux/arm64 (R4).
#   - capacity_provider_strategy mixes FARGATE (on-demand baseline) and
#     FARGATE_SPOT so that dev workloads can run mostly/entirely on Spot
#     capacity for further savings; see variables.tf and README for the
#     weight/base defaults.
#   - ECS tasks run in public subnets with assign_public_ip = true instead of
#     a NAT Gateway (R6); egress is restricted to the security group defined
#     in the network module.

resource "aws_ecs_task_definition" "this" {
  family                   = "${var.name_prefix}-${var.service_name}"
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
      name      = var.service_name
      image     = local.container_image
      essential = true

      portMappings = [
        {
          containerPort = var.container_port
          protocol      = "tcp"
        }
      ]

      # Caller-supplied plain env vars (e.g. PORT, DB_HOST, ISSUER) and
      # Secrets Manager-backed secrets (e.g. DB_CREDENTIALS); which
      # env/secrets to inject differs per service (see envs/dev/main.tf), so
      # this module stays a thin pass-through and never hardcodes an
      # application's variable names.
      environment = var.environment
      secrets     = var.secrets

      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.this.name
          "awslogs-region"        = data.aws_region.current.name
          "awslogs-stream-prefix" = var.service_name
        }
      }
    }
  ])

  tags = merge(var.tags, { Name = "${var.name_prefix}-${var.service_name}" })
}

resource "aws_ecs_service" "this" {
  name            = "${var.name_prefix}-${var.service_name}"
  cluster         = var.ecs_cluster_id
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
    target_group_arn = aws_lb_target_group.this.arn
    container_name   = var.service_name
    container_port   = var.container_port
  }

  # Ensure the listener rule (and therefore the listener/target group) exists
  # before the service tries to register tasks with the target group.
  depends_on = [aws_lb_listener_rule.this]

  tags = merge(var.tags, { Name = "${var.name_prefix}-${var.service_name}" })
}
