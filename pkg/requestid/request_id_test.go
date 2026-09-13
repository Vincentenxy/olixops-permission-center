package requestid_test

import (
	"context"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
	"olixops-permission-center/pkg/logger"
	"olixops-permission-center/pkg/requestid"
)

func TestEnsurePreservesTracePrecedence(t *testing.T) {
	for _, test := range []struct {
		name     string
		existing string
		supplied string
		want     string
	}{
		{name: "supplied", supplied: "Trace.Mixed-123", want: "Trace.Mixed-123"},
		{name: "existing", existing: "existing-trace", want: "existing-trace"},
		{name: "supplied wins", existing: "existing-trace", supplied: "upstream-trace", want: "upstream-trace"},
	} {
		t.Run(test.name, func(t *testing.T) {
			parent := context.WithValue(context.Background(), logger.TraceIDKey, test.existing)
			ctx, id := requestid.Ensure(parent, test.supplied)
			if id != test.want || requestid.FromContext(ctx) != test.want {
				t.Fatalf("trace = %q, context trace = %q; want %q", id, requestid.FromContext(ctx), test.want)
			}
			if requestid.FromContext(parent) != test.existing {
				t.Fatal("the parent context was changed")
			}
		})
	}
}

func TestEnsureGeneratesIndependentIdentifiers(t *testing.T) {
	seen := make(map[string]struct{})
	for range 32 {
		ctx, id := requestid.Ensure(context.Background(), "")
		decoded, err := hex.DecodeString(id)
		if err != nil || len(decoded) != 16 || requestid.FromContext(ctx) != id {
			t.Fatalf("invalid generated trace %q", id)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("independent requests reused trace %q", id)
		}
		seen[id] = struct{}{}
		next, reused := requestid.Ensure(ctx, "")
		if reused != id || requestid.FromContext(next) != id {
			t.Fatal("a downstream adapter replaced the generated trace")
		}
	}
}

func TestEnsureReplacesInvalidIdentifiers(t *testing.T) {
	for _, value := range []string{strings.Repeat("x", 129), "line\nbreak", "null\x00byte", "white space", "中文", "\x7f"} {
		t.Run(hex.EncodeToString([]byte(value)), func(t *testing.T) {
			parent := context.WithValue(context.Background(), logger.TraceIDKey, value)
			ctx, id := requestid.Ensure(parent, value)
			decoded, err := hex.DecodeString(id)
			if err != nil || len(decoded) != 16 || requestid.FromContext(ctx) != id {
				t.Fatal("invalid supplied and context values were not replaced")
			}
			parent = context.WithValue(context.Background(), logger.TraceIDKey, "existing-valid")
			_, id = requestid.Ensure(parent, value)
			if id != "existing-valid" {
				t.Fatal("invalid supplied value did not fall back to valid context")
			}
		})
	}
	boundary := strings.Repeat("x", requestid.MaxLength)
	_, id := requestid.Ensure(context.Background(), boundary)
	if id != boundary {
		t.Fatal("valid maximum-length identifier was changed")
	}
}

func TestEnsurePreservesContextLifecycle(t *testing.T) {
	type testKey struct{}
	deadline := time.Now().Add(time.Minute)
	parent, cancel := context.WithDeadline(context.WithValue(context.Background(), testKey{}, "retained"), deadline)
	defer cancel()
	ctx, _ := requestid.Ensure(parent, "trace")
	if got, ok := ctx.Deadline(); !ok || !got.Equal(deadline) {
		t.Fatal("the parent deadline was lost")
	}
	if ctx.Value(testKey{}) != "retained" {
		t.Fatal("the parent values were lost")
	}
	cancel()
	if ctx.Err() != context.Canceled {
		t.Fatal("parent cancellation was not propagated")
	}
}

func TestFromContextWithoutIdentifier(t *testing.T) {
	for _, ctx := range []context.Context{
		nil,
		context.Background(),
		context.WithValue(context.Background(), logger.TraceIDKey, 123),
	} {
		if id := requestid.FromContext(ctx); id != "" {
			t.Fatalf("unexpected trace %q", id)
		}
	}
}

func TestEnsuredTraceIsVisibleToLogger(t *testing.T) {
	core, entries := observer.New(zap.InfoLevel)
	previous := logger.L()
	logger.Replace(zap.New(core))
	t.Cleanup(func() { logger.Replace(previous) })
	ctx, id := requestid.Ensure(context.Background(), "upstream-trace")
	logger.Info(ctx, "request completed")
	logs := entries.All()
	if len(logs) != 1 || logs[0].ContextMap()["trace-id"] != id {
		t.Fatal("the shared trace identifier was not included in the logger output")
	}
}
