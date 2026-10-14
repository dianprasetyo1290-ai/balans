// Command healthcheck dipakai healthcheck Docker karena image distroless tidak memiliki curl/wget.
// Exit 0 jika GET http://127.0.0.1:<HEALTHCHECK_PORT>/healthz mengembalikan 200.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"
)

const defaultPort = 8080

func main() {
	os.Exit(run())
}

func run() int {
	port := defaultPort
	if v := os.Getenv("HEALTHCHECK_PORT"); v != "" {
		p, err := strconv.Atoi(v)
		if err != nil || p <= 0 || p > 65535 {
			return 1
		}
		port = p
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	url := fmt.Sprintf("http://127.0.0.1:%d/healthz", port)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil) //nolint:gosec // loopback dengan port numerik
	if err != nil {
		return 1
	}
	resp, err := http.DefaultClient.Do(req) //nolint:gosec // tujuan selalu loopback dengan port numerik
	if err != nil {
		return 1
	}
	defer resp.Body.Close() //nolint:errcheck // body tidak dibaca
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
