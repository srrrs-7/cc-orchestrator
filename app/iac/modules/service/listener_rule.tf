# Listener rule attached to the shared ALB HTTP listener (modules/platform).
#
# CloudFront strips the leading /api or /auth path segment before forwarding
# to the ALB origin (see modules/cdn's strip_prefix Function), so requests
# reach the ALB with root-relative paths regardless of which service they
# were originally addressed to -- paths alone cannot distinguish api from
# auth at the ALB. Routing is therefore header-based: var.route_conditions
# carries one or more {header_name, values} pairs that must ALL match
# (multiple `condition` blocks on the same rule are ANDed by the ALB) for
# the rule to forward to this service's target group. For example:
#   - api:  [{X-Origin-Verify = <secret>}]
#   - auth: [{X-Origin-Verify = <secret>}, {X-Target-Service = "auth"}]
# Combined with the ALB security group's CloudFront-prefix-list-only ingress
# rule (network module), the X-Origin-Verify condition gives a two-layer
# defense against direct ALB access (R3); X-Target-Service alone is a
# routing discriminator, not a security boundary.

resource "aws_lb_listener_rule" "this" {
  listener_arn = var.listener_arn
  priority     = var.listener_priority

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.this.arn
  }

  dynamic "condition" {
    for_each = var.route_conditions

    content {
      http_header {
        http_header_name = condition.value.header_name
        values           = condition.value.values
      }
    }
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-${var.service_name}-route-rule" })
}
