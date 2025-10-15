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

	time.Sleep(2 * time.Second)

	resp, err := http.Get("http://localhost:18080/health")
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}
}
