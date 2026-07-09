// CloudFront Function (viewer-request, runtime cloudfront-js-2.0).
//
// Strips the leading path segment from the request URI before it reaches
// the ALB origin. Shared by the /api/* and /auth/* ordered_cache_behaviors
// (see modules/cdn/main.tf): "/api/tasks" -> "/tasks",
// "/auth/.well-known/openid-configuration" -> "/.well-known/openid-configuration",
// "/api" -> "/", "/auth/" -> "/".
//
// This lets app/api and app/auth stay unmodified (both register their
// routes at the container root, see docs/plans/SPEC-004-plan.md "R5 の確定
// 結論"): CloudFront exposes them under /api and /auth respectively, but the
// containers themselves never see those prefixes.
function handler(event) {
  var request = event.request;
  var uri = request.uri;

  var match = uri.match(/^\/[^/]+(\/.*)?$/);
  request.uri = match && match[1] ? match[1] : "/";

  return request;
}
