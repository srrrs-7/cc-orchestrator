package route

import "net/http"

// securityHeadersCSP is the Content-Security-Policy applied to every
// response (ISSUE-042). It is deliberately as strict as possible while
// still allowing the server-rendered login/consent HTML
// (route/templates/login.html, route/templates/consent.html) to work:
//
//   - default-src 'none': nothing loads unless explicitly allowed below.
//     Both templates are self-contained (no external scripts, images,
//     fonts, or stylesheets), and neither uses inline <script> or
//     script-src-requiring constructs, so no script-src exception is
//     needed.
//   - style-src 'self' 'unsafe-inline': both templates use an inline
//     <style> block in <head> (no external stylesheet), which requires
//     'unsafe-inline' (there is no nonce/hash infrastructure here); 'self'
//     is added in case a future revision moves the CSS to a same-origin
//     file.
//   - form-action 'self': login/consent forms POST back to the same
//     origin (method="post" action="").
//   - base-uri 'none': neither template uses <base>; prevents a future
//     injected <base> tag from redirecting relative URLs off-origin.
//   - frame-ancestors 'none': the actual clickjacking fix (ISSUE-042) --
//     no origin, including this server's own, may embed these pages in a
//     frame/iframe/object. Restated by X-Frame-Options: DENY below for
//     browsers that only honor the legacy header.
const securityHeadersCSP = "default-src 'none'; style-src 'self' 'unsafe-inline'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'"

// securityHeaders is middleware that sets response headers hardening
// every endpoint against clickjacking and content-sniffing (ISSUE-042).
// It wraps the whole router (see NewRouter) rather than individual
// handlers so protection does not depend on nginx/CloudFront in front
// of this server, which the production /auth/* path bypasses.
//
// The headers are harmless on the JSON API endpoints (/token,
// /userinfo, /.well-known/*, /admin/*, ...): browsers do not act on
// frame-ancestors/X-Frame-Options for non-HTML responses, and nosniff /
// Referrer-Policy are always safe to send. Applying them uniformly
// avoids having to special-case which handlers are "HTML" ones.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		// Legacy clickjacking defense (IE/older browsers that do not
		// honor CSP frame-ancestors); frame-ancestors above is the
		// modern, more expressive equivalent and takes precedence in
		// browsers that support both.
		h.Set("X-Frame-Options", "DENY")
		h.Set("Content-Security-Policy", securityHeadersCSP)
		// Prevents MIME-sniffing a response away from its declared
		// Content-Type (e.g. treating a JSON error body as HTML).
		h.Set("X-Content-Type-Options", "nosniff")
		// Sends the full referrer only on same-origin navigations;
		// cross-origin requests get origin-only (still useful for
		// analytics/debugging) rather than the full URL (which could
		// leak query parameters such as an authorization `state`).
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")

		next.ServeHTTP(w, r)
	})
}
