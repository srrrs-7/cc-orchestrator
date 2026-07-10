// main_test.go exercises newServer (main.go), the helper the ISSUE-024
// (gosec G112) hardening extracted so the *http.Server this process
// listens with can be unit-tested without starting a real listener.
// Go's zero-value http.Server leaves ReadHeaderTimeout/ReadTimeout/
// WriteTimeout/IdleTimeout unbounded, which is exactly the
// Slowloris-style exposure gosec G112 flags. These are regression
// tests: expected values are hardcoded (not read back from the
// package's own readHeaderTimeout/readTimeout/writeTimeout/
// idleTimeout constants) so that an accidental future edit to one of
// those constants -- e.g. reverting it to 0 -- fails this test instead
// of trivially agreeing with itself.
package main

import (
	"net/http"
	"testing"
	"time"
)

// TestNewServer_Timeouts is the 正常系 case: newServer must set all
// four timeouts to their documented, non-zero values.
func TestNewServer_Timeouts(t *testing.T) {
	handler := http.NewServeMux()
	srv := newServer(":8080", handler)

	cases := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"ReadHeaderTimeout", srv.ReadHeaderTimeout, 5 * time.Second},
		{"ReadTimeout", srv.ReadTimeout, 10 * time.Second},
		{"WriteTimeout", srv.WriteTimeout, 10 * time.Second},
		{"IdleTimeout", srv.IdleTimeout, 60 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// 境界値: zero is Go's unbounded default -- the exact
			// condition G112 flags -- so it must never be produced,
			// regardless of what the "want" value is.
			if tc.got == 0 {
				t.Fatalf("newServer().%s = 0, want non-zero (G112: unbounded timeout)", tc.name)
			}
			if tc.got != tc.want {
				t.Errorf("newServer().%s = %v, want %v", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestNewServer_AddrAndHandler is a light sanity check that newServer
// threads its two parameters through unchanged, alongside the fixed
// timeouts asserted above.
func TestNewServer_AddrAndHandler(t *testing.T) {
	const addr = ":9999"
	handler := http.NewServeMux()

	srv := newServer(addr, handler)

	if srv.Addr != addr {
		t.Errorf("newServer().Addr = %q, want %q", srv.Addr, addr)
	}
	gotHandler, ok := srv.Handler.(*http.ServeMux)
	if !ok || gotHandler != handler {
		t.Errorf("newServer().Handler = %v, want the *http.ServeMux passed in", srv.Handler)
	}
}
