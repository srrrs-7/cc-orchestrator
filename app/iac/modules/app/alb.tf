# ALB, target group, HTTP listener and the custom-header verification rule.
#
# The listener's default action returns a fixed 403 response; only requests
# carrying the expected origin-verify header (injected by CloudFront as a
# custom origin header, see the cdn module) are forwarded to the target
# group. Combined with the ALB security group's CloudFront-prefix-list-only
# ingress rule (network module), this gives a two-layer defense against
# direct ALB access (R3).

resource "aws_lb" "this" {
  name               = "${var.name_prefix}-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [var.alb_sg_id]
  subnets            = var.public_subnet_ids

  # Sample/dev environment: allow easy teardown.
  enable_deletion_protection = false

  tags = merge(var.tags, { Name = "${var.name_prefix}-alb" })
}

resource "aws_lb_target_group" "app" {
  name        = "${var.name_prefix}-tg"
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

  tags = merge(var.tags, { Name = "${var.name_prefix}-tg" })
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  # Default-deny: anything without the verified origin header gets a fixed
  # 403 instead of being forwarded to the target group.
  default_action {
    type = "fixed-response"

    fixed_response {
      content_type = "text/plain"
      message_body = "Forbidden"
      status_code  = "403"
    }
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-http-listener" })
}

resource "aws_lb_listener_rule" "verified_origin" {
  listener_arn = aws_lb_listener.http.arn
  priority     = 10

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.app.arn
  }

  condition {
    http_header {
      http_header_name = var.origin_verify_header_name
      values           = [var.origin_verify_header_value]
    }
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-verified-origin-rule" })
}
