package route

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthRoute_Unauthenticated(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	RegisterHealthRoute(mux)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /health status = %d, want %d", rec.Code, http.StatusOK)
	}
	if body := rec.Body.String(); body != `{"status":"ok"}`+"\n" {
		t.Fatalf("GET /health body = %q, want %q", body, `{"status":"ok"}`+"\n")
	}
}
