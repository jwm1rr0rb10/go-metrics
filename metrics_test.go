package metrics

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestNewServer_InvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr string
	}{
		{
			name:    "EmptyHost",
			cfg:     NewConfig(WithHost("")),
			wantErr: "host cannot be empty",
		},
		{
			name:    "ZeroPort",
			cfg:     NewConfig(WithPort(0)),
			wantErr: "port cannot be zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewServer(tt.cfg)
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("Expected error: %v, got: %v", tt.wantErr, err)
			}
		})
	}
}

func TestServerLifecycle(t *testing.T) {
	cfg := NewConfig()
	server, err := NewServer(cfg)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := server.Run(ctx); err != nil && err != http.ErrServerClosed {
			t.Error("Server failed:", err)
		}
	}()

	time.Sleep(100 * time.Millisecond) // Wait for server to start

	resp, err := http.Get("http://" + cfg.address + "/metrics")
	if err != nil {
		t.Fatal("HTTP request failed:", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	if err := server.Close(); err != nil {
		t.Error("Failed to close server:", err)
	}
}
