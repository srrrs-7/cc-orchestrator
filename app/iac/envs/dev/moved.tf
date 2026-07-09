# Non-regression for the modules/app -> modules/platform + modules/service
# split (SPEC-004). Every resource that previously lived under module.app
# is mapped to its new address so `terraform plan` shows these as moves
# (no destroy/create), not a replacement of the existing api deployment.
#
# module.app resources map to module.platform (ALB/listener/cluster, shared)
# and module.service_api (everything else, api-specific). See
# docs/plans/SPEC-004-plan.md "moved ブロック(非退行の担保)".

# --- modules/platform (shared ALB / listener / ECS cluster) -----------------

moved {
  from = module.app.aws_lb.this
  to   = module.platform.aws_lb.this
}

moved {
  from = module.app.aws_lb_listener.http
  to   = module.platform.aws_lb_listener.http
}

moved {
  from = module.app.aws_ecs_cluster.this
  to   = module.platform.aws_ecs_cluster.this
}

moved {
  from = module.app.aws_ecs_cluster_capacity_providers.this
  to   = module.platform.aws_ecs_cluster_capacity_providers.this
}

# --- module.service_api (api-specific: target group, listener rule, ECR,
# task definition, ECS service, IAM, logs) ------------------------------------

moved {
  from = module.app.aws_lb_target_group.app
  to   = module.service_api.aws_lb_target_group.this
}

moved {
  from = module.app.aws_lb_listener_rule.verified_origin
  to   = module.service_api.aws_lb_listener_rule.this
}

moved {
  from = module.app.aws_ecr_repository.this
  to   = module.service_api.aws_ecr_repository.this
}

moved {
  from = module.app.aws_ecr_lifecycle_policy.this
  to   = module.service_api.aws_ecr_lifecycle_policy.this
}

moved {
  from = module.app.aws_cloudwatch_log_group.this
  to   = module.service_api.aws_cloudwatch_log_group.this
}

moved {
  from = module.app.aws_ecs_task_definition.this
  to   = module.service_api.aws_ecs_task_definition.this
}

moved {
  from = module.app.aws_ecs_service.this
  to   = module.service_api.aws_ecs_service.this
}

moved {
  from = module.app.aws_iam_role.task_execution
  to   = module.service_api.aws_iam_role.task_execution
}

moved {
  from = module.app.aws_iam_role_policy_attachment.task_execution_managed
  to   = module.service_api.aws_iam_role_policy_attachment.task_execution_managed
}

# task_execution_secrets became a for_each-keyed resource in modules/service
# (only created when var.secret_read_arns is non-empty, keyed "secrets");
# `moved` supports this no-index -> for_each-index transition.
moved {
  from = module.app.aws_iam_role_policy.task_execution_secrets
  to   = module.service_api.aws_iam_role_policy.task_execution_secrets["secrets"]
}

moved {
  from = module.app.aws_iam_role.task
  to   = module.service_api.aws_iam_role.task
}
