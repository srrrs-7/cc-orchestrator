# ECR repository for this service's container image (one repository per
# service instance, e.g. "<name_prefix>-api" / "<name_prefix>-auth").
# Building/pushing the image is out of scope for this module (see README);
# only the repository itself is provisioned.

resource "aws_ecr_repository" "this" {
  name                 = "${var.name_prefix}-${var.service_name}"
  image_tag_mutability = "MUTABLE"

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-${var.service_name}" })
}

resource "aws_ecr_lifecycle_policy" "this" {
  repository = aws_ecr_repository.this.name

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
