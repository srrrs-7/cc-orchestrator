# Shared ECR repository for the app/migrator image (SPEC-005 RF.1.2). Unlike
# api/auth's own per-service ECR repositories (modules/service/ecr.tf), there
# is exactly one migrator image: a single Go binary that both
# module.service_api and module.service_auth's migration init containers
# pull, distinguished only by the `-target api` / `-target auth` command
# passed via migration_command (see main.tf's migration_image /
# migration_command on both module.service_* calls). Kept as a plain
# resource here rather than a new modules/migrator wrapper because it is a
# single ECR repository with no other resources attached (no IAM role, no
# ECS service of its own) -- introducing a module for one resource would add
# indirection without reuse benefit (this repository is only ever called
# from envs/dev).
#
# As with api/auth's own repositories, building/pushing the image is out of
# scope for Terraform (see envs/dev/README.md "コンテナイメージについて" and
# "Postgres 永続化・マイグレーション init コンテナについて"): until an image
# is pushed to this repository's ":latest" tag, both services' migration
# init containers fail to pull it and their deployments cannot roll out.

resource "aws_ecr_repository" "migrator" {
  name                 = "${local.name_prefix}-migrator"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }

  tags = merge(local.common_tags, { Name = "${local.name_prefix}-migrator" })
}

resource "aws_ecr_lifecycle_policy" "migrator" {
  repository = aws_ecr_repository.migrator.name

  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Expire untagged images after 14 days"
        selection = {
          tagStatus   = "untagged"
          countType   = "sinceImagePushed"
          countUnit   = "days"
          countNumber = 14
        }
        action = { type = "expire" }
      }
    ]
  })
}
