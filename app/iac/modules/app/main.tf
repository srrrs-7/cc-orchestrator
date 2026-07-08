# app module
#
# ALB + Target Group + Listener/Listener rule, ECR repository, ECS
# cluster/task definition/service, IAM roles, and CloudWatch Logs for the
# app/api Fargate service. See alb.tf / ecr.tf / ecs.tf / iam.tf / logs.tf for
# the resource definitions, grouped by responsibility.

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
