package main

import (
	"net/http"
	"os"
	"testing"
	"time"
)

func TestServerStarts(t *testing.T) {
	os.Setenv("PORT", "18080")
	os.Setenv("DATA_PATH", "./test_data")
	os.Setenv("MEDIA_PATH", "./test_data/media")
	os.Setenv("ADMIN_USER", "testadmin")
	os.Setenv("ADMIN_PASS", "testpass")

	defer os.RemoveAll("./test_data")

	go main()

	// main() runs PreloadData() (network-bound: GitHub stats, repo collection)
	// before it starts listening, so the port can take a while to open in CI.
	// Poll /health instead of assuming a fixed startup time.
	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(60 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get("http://localhost:18080/health")
		if err != nil {
			lastErr = err
			time.Sleep(300 * time.Millisecond)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200 from /health, got %d", resp.StatusCode)
		}
		return // server is up and healthy
	}
	t.Fatalf("server did not become healthy within 60s: %v", lastErr)
}
