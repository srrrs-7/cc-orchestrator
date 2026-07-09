# ALB target group for this service instance. The ALB, HTTP listener and ECS
# cluster themselves live in modules/platform (shared across all service
# instances registered against the same listener); this module only attaches
# a target group + listener rule to the listener ARN it is given (see
# listener_rule.tf).

resource "aws_lb_target_group" "this" {
  name        = "${var.name_prefix}-${var.service_name}-tg"
  port        = var.container_port
  protocol    = "HTTP"
  vpc_id      = var.vpc_id
  target_type = "ip"

  health_check {
    enabled             = true
    path                = var.health_check_path
    protocol            = "HTTP"
    matcher             = "200"
    interval            = 30
    timeout             = 5
    healthy_threshold   = 2
    unhealthy_threshold = 3
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-${var.service_name}-tg" })
}
