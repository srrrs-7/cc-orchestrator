# CloudFront Functions (lightweight viewer-request JS, cheaper/faster than
# Lambda@Edge for this kind of pure URI rewrite). See functions/*.js for the
# implementations and modules/cdn/README.md for why each was chosen over the
# alternatives (distribution-wide custom_error_response for SPA fallback,
# app/auth or app/api base-path support for the prefix strip).

resource "aws_cloudfront_function" "strip_prefix" {
  name    = "${var.name_prefix}-strip-prefix"
  runtime = "cloudfront-js-2.0"
  comment = "Strips the leading /api or /auth path segment before forwarding to the ALB origin."
  publish = true
  code    = file("${path.module}/functions/strip_prefix.js")
}

resource "aws_cloudfront_function" "spa_fallback" {
  name    = "${var.name_prefix}-spa-fallback"
  runtime = "cloudfront-js-2.0"
  comment = "Rewrites extensionless client-side-route paths to /index.html for the S3/web origin (SPA fallback)."
  publish = true
  code    = file("${path.module}/functions/spa_fallback.js")
}
