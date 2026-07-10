# Shared ALB and its HTTP listener.
#
# The listener's default action returns a fixed 403 response; per-service
# listener rules (defined in modules/service, attached via this module's
# listener_arn output) forward matching requests to each service's own
# target group based on custom headers (see modules/service/listener_rule.tf
# for why header-based routing is needed instead of path-based routing).
# Combined with the ALB security group's CloudFront-prefix-list-only ingress
# rule (network module), this gives a two-layer defense against direct ALB
# access (R3).

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

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  # Default-deny: anything that doesn't match one of the per-service listener
  # rules attached by modules/service gets a fixed 403 instead of being
  # forwarded anywhere.
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
