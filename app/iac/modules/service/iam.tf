# IAM roles for the ECS task: an execution role (used by the ECS agent to
# pull the image, write logs and resolve task secrets) and a task role (used
# by the running application; kept minimal since neither app/api nor
# app/auth currently calls any AWS API at runtime).

data "aws_iam_policy_document" "ecs_tasks_assume" {
  statement {
    actions = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "task_execution" {
  name               = "${var.name_prefix}-${var.service_name}-ecs-task-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume.json

  tags = merge(var.tags, { Name = "${var.name_prefix}-${var.service_name}-ecs-task-execution" })
}

resource "aws_iam_role_policy_attachment" "task_execution_managed" {
  role       = aws_iam_role.task_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# Only granted when the caller supplies secret ARNs to read (e.g. api's RDS
# master user secret). auth currently has none, so no inline policy/data
# source is created for it. for_each (rather than count) is used so the
# resource's existence doesn't shift indices across plans (see
# .claude/rules/iac.md).
data "aws_iam_policy_document" "task_execution_secrets" {
  for_each = length(var.secret_read_arns) > 0 ? toset(["secrets"]) : toset([])

  statement {
    sid       = "ReadTaskSecrets"
    actions   = ["secretsmanager:GetSecretValue"]
    resources = var.secret_read_arns
  }
}

resource "aws_iam_role_policy" "task_execution_secrets" {
  for_each = data.aws_iam_policy_document.task_execution_secrets

  name   = "${var.name_prefix}-${var.service_name}-secrets-read"
  role   = aws_iam_role.task_execution.id
  policy = each.value.json
}

resource "aws_iam_role" "task" {
  name               = "${var.name_prefix}-${var.service_name}-ecs-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_tasks_assume.json

  tags = merge(var.tags, { Name = "${var.name_prefix}-${var.service_name}-ecs-task" })
}
