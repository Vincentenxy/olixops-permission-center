package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBootstrapWithoutExternalDependenciesStopsOnCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)
	t.Setenv("HTTP_ADDR", "127.0.0.1:0")
	t.Setenv("GRPC_ADDR", "127.0.0.1:0")
	t.Setenv("DATABASE_DSN", "")
	t.Setenv("REDIS_ADDR", "")
	t.Setenv("SPICEDB_ENABLED", "false")
	t.Setenv("APP_ENV", "release")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Run(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("bootstrap did not stop after cancellation")
	}
}
