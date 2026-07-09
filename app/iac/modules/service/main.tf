# service module
#
# Generic, reusable "one Fargate service behind the shared ALB" building
# block: target group + listener rule (target_group.tf / listener_rule.tf),
# ECR repository (ecr.tf), CloudWatch Logs (logs.tf), IAM roles (iam.tf) and
# ECS task definition/service (ecs.tf). The shared ALB, HTTP listener and ECS
# cluster live in modules/platform and are passed in via var.listener_arn /
# var.ecs_cluster_id. Called once per application (e.g. api, auth) with
# different var.service_name / var.route_conditions / var.environment /
# var.secrets values -- see envs/dev/main.tf for the two call sites.

terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

data "aws_region" "current" {}

locals {
  # var.container_image defaults to "" so that, unless overridden, the task
  # definition points at this module's own ECR repository (":latest"). No
  # image is built/pushed by Terraform (out of scope, see README): the ECS
  # service will not reach a healthy state until an image is pushed.
  container_image = var.container_image != "" ? var.container_image : "${aws_ecr_repository.this.repository_url}:latest"
}
