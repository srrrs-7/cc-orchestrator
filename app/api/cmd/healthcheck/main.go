// Command healthcheck is a minimal Docker HEALTHCHECK probe for the api
// container. It issues a GET request to a URL and exits 0 if the
// response status is 2xx, or 1 otherwise (network error, timeout, or
// non-2xx status). It exists because the distroless runtime image has
// no shell, curl, or wget.
package main

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

const (
	defaultURL = "http://localhost:8080/tasks"
	timeout    = 3 * time.Second
)

func main() {
	os.Exit(run())
}

func run() int {
	url := defaultURL
	if len(os.Args) > 1 {
		url = os.Args[1]
	} else if v := os.Getenv("HEALTHCHECK_URL"); v != "" {
		url = v
	}

	client := &http.Client{Timeout: timeout}

	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: request failed: %v\n", err)
		return 1
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		fmt.Fprintf(os.Stderr, "healthcheck: unhealthy status: %d\n", resp.StatusCode)
		return 1
	}

	return 0
}
