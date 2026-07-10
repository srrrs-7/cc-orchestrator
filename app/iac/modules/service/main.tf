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
      version = "~> 6.54"
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

  # ForceNew resource names (see variables.tf for why each is individually
  # overridable): default to "<name_prefix>-<service_name>-*" for a fresh
  # service instance, but a caller can pin any of them to an existing name to
  # avoid a replace when this module is reused for a previously-differently-
  # named resource (e.g. api's names inherited verbatim from the pre-SPEC-004
  # modules/app, see envs/dev/main.tf and moved.tf).
  target_group_name        = var.target_group_name != null ? var.target_group_name : "${var.name_prefix}-${var.service_name}-tg"
  task_execution_role_name = var.task_execution_role_name != null ? var.task_execution_role_name : "${var.name_prefix}-${var.service_name}-ecs-task-execution"
  task_role_name           = var.task_role_name != null ? var.task_role_name : "${var.name_prefix}-${var.service_name}-ecs-task"
  secrets_policy_name      = var.secrets_policy_name != null ? var.secrets_policy_name : "${var.name_prefix}-${var.service_name}-secrets-read"
}
