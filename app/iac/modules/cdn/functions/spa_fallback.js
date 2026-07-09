// CloudFront Function (viewer-request, runtime cloudfront-js-2.0).
//
// Attached only to the default (S3/web) cache behavior (see
// modules/cdn/main.tf) -- never to /api/* or /auth/*, so it cannot turn a
// genuine API/OIDC 403/404 into the SPA's index.html (that risk is why the
// plan rejects a distribution-wide custom_error_response instead, see
// docs/plans/SPEC-004-plan.md).
//
// Rewrites extensionless paths (client-side routes such as "/tasks/42",
// which do not exist as objects in the S3 bucket) to "/index.html" so the
// SPA's client-side router can resolve them, equivalent to nginx's
// `try_files $uri /index.html` (see app/web/nginx.conf, used only for local
// compose -- AWS has no nginx, see modules/cdn/README.md).
function handler(event) {
  var request = event.request;
  var uri = request.uri;

  var lastSegment = uri.substring(uri.lastIndexOf("/") + 1);
  if (lastSegment.indexOf(".") === -1) {
    request.uri = "/index.html";
  }

  return request;
}
