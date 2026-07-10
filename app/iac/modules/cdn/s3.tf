# Private S3 bucket holding the web SPA's built static assets
# (app/web `bun run build` output), served exclusively through this
# CloudFront distribution via Origin Access Control (OAC). All public access
# is blocked at the bucket level; the bucket policy additionally scopes
# reads to this specific distribution's ARN (AWS:SourceArn condition) so
# that no other CloudFront distribution -- even one an attacker controls --
# can read the bucket via OAC.
#
# NOTE: the bucket name is derived solely from var.name_prefix (no extra
# suffix), matching the plan's variable list. S3 bucket names must be
# globally unique across ALL AWS accounts, so a collision is possible; if
# `terraform apply` fails with BucketAlreadyExists, change var.name_prefix
# (e.g. add an account-specific suffix) before retrying. See
# docs/plans/SPEC-004-plan.md リスク "AWS アカウント ID / リージョン /
# backend バケット".

resource "aws_s3_bucket" "web" {
  bucket = "${var.name_prefix}-web"

  tags = merge(var.tags, { Name = "${var.name_prefix}-web" })
}

resource "aws_s3_bucket_public_access_block" "web" {
  bucket = aws_s3_bucket.web.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "web" {
  bucket = aws_s3_bucket.web.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_cloudfront_origin_access_control" "web" {
  name                              = "${var.name_prefix}-web-oac"
  description                       = "OAC for the web SPA S3 origin"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

# Grants CloudFront's OAC service principal read-only access, scoped to
# requests originating from this exact distribution (not just "any
# CloudFront distribution in this account").
data "aws_iam_policy_document" "web_bucket" {
  statement {
    sid       = "AllowCloudFrontServicePrincipalReadOnly"
    actions   = ["s3:GetObject"]
    resources = ["${aws_s3_bucket.web.arn}/*"]

    principals {
      type        = "Service"
      identifiers = ["cloudfront.amazonaws.com"]
    }

    condition {
      test     = "StringEquals"
      variable = "AWS:SourceArn"
      values   = [aws_cloudfront_distribution.this.arn]
    }
  }
}

resource "aws_s3_bucket_policy" "web" {
  bucket = aws_s3_bucket.web.id
  policy = data.aws_iam_policy_document.web_bucket.json
}
