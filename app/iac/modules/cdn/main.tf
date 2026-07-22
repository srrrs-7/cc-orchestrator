# cdn module
#
# WAFv2 Web ACL (CLOUDFRONT scope, must be created in us-east-1) and the
# single CloudFront distribution fronting all three origins: the web SPA's
# S3 bucket (default behavior), and the shared ALB for api (/api/*) and auth
# (/auth/*). The default (non-aliased) aws provider creates the CloudFront
# distribution and S3 resources (see s3.tf) -- both global/regional
# resources managed from any region -- while the aws.us_east_1 alias
# provider creates the Web ACL (CLOUDFRONT-scope WAFv2 is us-east-1 only).

terraform {
  required_providers {
    aws = {
      source                = "hashicorp/aws"
      version               = "~> 6.55"
      configuration_aliases = [aws.us_east_1]
    }
  }
}

# ---------------------------------------------------------------------------
# WAFv2 Web ACL (CLOUDFRONT scope; must be created via the us-east-1 region)
# ---------------------------------------------------------------------------

resource "aws_wafv2_web_acl" "this" {
  provider = aws.us_east_1

  name        = "${var.name_prefix}-waf"
  description = "WAF for ${var.name_prefix} CloudFront distribution"
  scope       = "CLOUDFRONT"

  default_action {
    allow {}
  }

  rule {
    name     = "aws-managed-common-rule-set"
    priority = 0

    override_action {
      none {}
    }

    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesCommonRuleSet"
        vendor_name = "AWS"
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${var.name_prefix}-common-rule-set"
      sampled_requests_enabled   = true
    }
  }

  rule {
    name     = "aws-managed-ip-reputation-list"
    priority = 1

    override_action {
      none {}
    }

    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesAmazonIpReputationList"
        vendor_name = "AWS"
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${var.name_prefix}-ip-reputation-list"
      sampled_requests_enabled   = true
    }
  }

  rule {
    name     = "rate-limit"
    priority = 2

    action {
      block {}
    }

    statement {
      rate_based_statement {
        limit              = var.waf_rate_limit
        aggregate_key_type = "IP"
      }
    }

    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${var.name_prefix}-rate-limit"
      sampled_requests_enabled   = true
    }
  }

  visibility_config {
    cloudwatch_metrics_enabled = true
    metric_name                = "${var.name_prefix}-waf"
    sampled_requests_enabled   = true
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-waf" })
}

# ---------------------------------------------------------------------------
# CloudFront distribution (default provider; CloudFront is a global service)
# ---------------------------------------------------------------------------

data "aws_cloudfront_cache_policy" "disabled" {
  name = "Managed-CachingDisabled"
}

data "aws_cloudfront_cache_policy" "caching_optimized" {
  name = "Managed-CachingOptimized"
}

data "aws_cloudfront_origin_request_policy" "all_viewer_except_host_header" {
  name = "Managed-AllViewerExceptHostHeader"
}

resource "aws_cloudfront_distribution" "this" {
  enabled             = true
  comment             = "${var.name_prefix} distribution (web / api / auth)"
  price_class         = var.price_class
  web_acl_id          = aws_wafv2_web_acl.this.arn
  default_root_object = "index.html"

  # --- S3 origin (web SPA static assets) --------------------------------------
  origin {
    domain_name              = aws_s3_bucket.web.bucket_regional_domain_name
    origin_id                = "s3-web"
    origin_access_control_id = aws_cloudfront_origin_access_control.web.id
  }

  # --- ALB origin for app/api (same ALB DNS as alb-auth, distinct origin_id
  # and custom headers so the ALB listener rule can tell them apart; see
  # modules/service/listener_rule.tf) ------------------------------------------
  origin {
    domain_name = var.alb_dns_name
    origin_id   = "alb-api"

    custom_origin_config {
      http_port                = 80
      https_port               = 443
      origin_protocol_policy   = "http-only"
      origin_ssl_protocols     = ["TLSv1.2"]
      origin_keepalive_timeout = 5
      origin_read_timeout      = 30
    }

    # Secret header verified by the ALB listener rule (modules/service) so
    # that only requests that actually passed through this CloudFront
    # distribution are forwarded past the ALB's default-403 action (R3).
    custom_header {
      name  = var.origin_verify_header_name
      value = var.origin_verify_header_value
    }
  }

  # --- ALB origin for app/auth (same ALB DNS as alb-api, plus an extra
  # X-Target-Service header so the ALB listener rule routes to the auth
  # target group instead of api's) ---------------------------------------------
  origin {
    domain_name = var.alb_dns_name
    origin_id   = "alb-auth"

    custom_origin_config {
      http_port                = 80
      https_port               = 443
      origin_protocol_policy   = "http-only"
      origin_ssl_protocols     = ["TLSv1.2"]
      origin_keepalive_timeout = 5
      origin_read_timeout      = 30
    }

    custom_header {
      name  = var.origin_verify_header_name
      value = var.origin_verify_header_value
    }

    custom_header {
      name  = var.auth_route_header_name
      value = var.auth_route_header_value
    }
  }

  # --- default behavior: web SPA (S3) -----------------------------------------
  default_cache_behavior {
    allowed_methods        = ["GET", "HEAD"]
    cached_methods         = ["GET", "HEAD"]
    target_origin_id       = "s3-web"
    viewer_protocol_policy = "redirect-to-https"
    compress               = true

    cache_policy_id = data.aws_cloudfront_cache_policy.caching_optimized.id

    # SPA fallback: rewrites extensionless client-side-route paths to
    # /index.html. Scoped to this behavior only, so /api/* and /auth/*
    # responses (including genuine 403/404s) are never rewritten -- see
    # modules/cdn/README.md for why a distribution-wide
    # custom_error_response was rejected instead.
    function_association {
      event_type   = "viewer-request"
      function_arn = aws_cloudfront_function.spa_fallback.arn
    }
  }

  # --- /api/* -> ALB (app/api) -------------------------------------------------
  ordered_cache_behavior {
    path_pattern           = "/api/*"
    allowed_methods        = ["GET", "HEAD", "OPTIONS", "PUT", "POST", "PATCH", "DELETE"]
    cached_methods         = ["GET", "HEAD"]
    target_origin_id       = "alb-api"
    viewer_protocol_policy = "redirect-to-https"
    compress               = true

    # API responses are not cached by default; caching can be revisited per
    # endpoint if needed later.
    cache_policy_id          = data.aws_cloudfront_cache_policy.disabled.id
    origin_request_policy_id = data.aws_cloudfront_origin_request_policy.all_viewer_except_host_header.id

    # Strips the "/api" prefix so app/api (which registers routes at its
    # container root, e.g. "/tasks") receives the request unmodified; see
    # docs/plans/SPEC-004-plan.md "R5 の確定結論".
    function_association {
      event_type   = "viewer-request"
      function_arn = aws_cloudfront_function.strip_prefix.arn
    }
  }

  # --- /auth/* -> ALB (app/auth) -----------------------------------------------
  ordered_cache_behavior {
    path_pattern           = "/auth/*"
    allowed_methods        = ["GET", "HEAD", "OPTIONS", "PUT", "POST", "PATCH", "DELETE"]
    cached_methods         = ["GET", "HEAD"]
    target_origin_id       = "alb-auth"
    viewer_protocol_policy = "redirect-to-https"
    compress               = true

    # OIDC responses (discovery/authorize/token/userinfo/jwks) are not
    # cached; token exchange in particular must never be served from cache.
    cache_policy_id          = data.aws_cloudfront_cache_policy.disabled.id
    origin_request_policy_id = data.aws_cloudfront_origin_request_policy.all_viewer_except_host_header.id

    # Strips the "/auth" prefix so app/auth (which registers routes at its
    # container root, e.g. "/authorize", "/.well-known/jwks.json") receives
    # the request unmodified. allowed_methods includes POST because /token
    # is a POST endpoint.
    function_association {
      event_type   = "viewer-request"
      function_arn = aws_cloudfront_function.strip_prefix.arn
    }
  }

  restrictions {
    geo_restriction {
      restriction_type = "none"
    }
  }

  # No ACM certificate / custom domain in scope: use CloudFront's default
  # certificate on the default *.cloudfront.net domain.
  viewer_certificate {
    cloudfront_default_certificate = true
  }

  tags = merge(var.tags, { Name = "${var.name_prefix}-cdn" })
}
