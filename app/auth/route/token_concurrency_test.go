package route_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// TestToken_ConcurrentCodeReuse_ExactlyOneSucceeds covers the
// concurrency fix in AuthorizationService.Token /
// infra/memory.AuthCodeRepository.Consume: firing many concurrent
// /token requests for the same authorization code must yield exactly
// one 200 (a single successful redemption) and every other request
// must observe invalid_grant, never two successful token issuances
// for one code. Run with `go test -race` to also confirm the
// repository's Consume critical section has no data race.
func TestToken_ConcurrentCodeReuse_ExactlyOneSucceeds(t *testing.T) {
	h := newTestHandler(t)

	verifier := strings.Repeat("A", 43)
	code := issueAuthCode(t, h, pkceChallenge(verifier), "openid", "")

	form := url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {testRedirectURI},
		"client_id":     {testClientID},
		"code_verifier": {verifier},
	}.Encode()

	const n = 30
	statuses := make([]int, n)
	errorCodes := make([]string, n)

	var wg sync.WaitGroup
	wg.Add(n)
	start := make(chan struct{})

	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start // all goroutines fire together, no ordering guarantee

			req := httptest.NewRequest(http.MethodPost, "/token", strings.NewReader(form))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			// Each goroutine writes only to its own index i, so this is
			// race-free even though the slices are shared: there is no
			// overlapping access between goroutines.
			statuses[i] = rec.Code
			if rec.Code != http.StatusOK {
				var body struct {
					Error string `json:"error"`
				}
				_ = json.Unmarshal(rec.Body.Bytes(), &body)
				errorCodes[i] = body.Error
			}
		}()
	}
	close(start)
	wg.Wait()

	// All result inspection and t.Error* calls happen here, back on
	// the test's own goroutine, after every worker has finished.
	var successCount, invalidGrantCount int
	for i := 0; i < n; i++ {
		switch statuses[i] {
		case http.StatusOK:
			successCount++
		case http.StatusBadRequest:
			if errorCodes[i] == "invalid_grant" {
				invalidGrantCount++
			} else {
				t.Errorf("request %d: status %d with unexpected error %q, want invalid_grant", i, statuses[i], errorCodes[i])
			}
		default:
			t.Errorf("request %d: unexpected status %d", i, statuses[i])
		}
	}

	if successCount != 1 {
		t.Errorf("successCount = %d, want exactly 1 (single-use must hold under concurrency)", successCount)
	}
	if invalidGrantCount != n-1 {
		t.Errorf("invalidGrantCount = %d, want %d", invalidGrantCount, n-1)
	}
}
