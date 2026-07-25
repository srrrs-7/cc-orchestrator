package route

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// maxFormBodySize caps application/x-www-form-urlencoded bodies (token,
// revoke, introspect) to mitigate unbounded reads (ISSUE-010 parity).
const maxFormBodySize = 1 << 20 // 1 MiB

// parseFormBody parses an HTML form body with a size limit. On failure it
// writes an OAuth-style invalid_request (or body-too-large) response and
// returns false.
func parseFormBody(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBodySize)
	if err := r.ParseForm(); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeBadRequest(w, "request body too large")
			return false
		}
		writeBadRequest(w, "malformed form body")
		return false
	}
	return true
}

// decodeJSONBody decodes JSON from r.Body with maxFormBodySize cap.
func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBodySize)
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeBadRequest(w, "request body too large")
			return false
		}
		if errors.Is(err, io.EOF) {
			writeBadRequest(w, "request body must be valid JSON")
			return false
		}
		writeBadRequest(w, "request body must be valid JSON")
		return false
	}
	return true
}
