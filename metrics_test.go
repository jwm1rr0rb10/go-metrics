package metrics

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestNewServer_InvalidConfig(t *testing.T) {
	// ... (unchanged - your original tests are fine)
}

func TestServerLifecycle(t *testing.T) {
	cfg := NewConfig()
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- server.Run(ctx)
	}()

	// Wait for server to be ready
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get("http://" + cfg.address + "/metrics")
	if err != nil {
		t.Fatal("HTTP request failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	cancel() // trigger graceful shutdown
	if err := <-done; err != nil && err != context.Canceled {
		t.Error("Server failed:", err)
	}
}
