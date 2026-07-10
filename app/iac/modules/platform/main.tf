# platform module
#
# Infrastructure shared by every service instance created by
# modules/service: a single ALB + HTTP listener, and a single ECS cluster
# (with capacity providers). No target group, listener rule or ECS
# service/task lives here -- those are per-service and defined by
# modules/service, which attaches itself to var.listener_arn / this module's
# ecs_cluster_id output. See alb.tf / ecs.tf for the resource definitions.

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 6.54"
    }
  }
}
