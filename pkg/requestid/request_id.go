// Package requestid shares trace identifiers across transport adapters.
package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"

	"olixops-permission-center/pkg/logger"
)

// MaxLength bounds response metadata and persisted audit identifiers.
const MaxLength = 128

// Ensure preserves a valid supplied identifier, reuses a valid context value,
// or generates an identifier. Valid identifiers contain 1-128 visible ASCII
// bytes; malformed values are replaced, never truncated. ctx must not be nil.
func Ensure(ctx context.Context, supplied string) (context.Context, string) {
	if !valid(supplied) {
		supplied = FromContext(ctx)
	}
	if !valid(supplied) {
		var bytes [16]byte
		// Go 1.26 guarantees that rand.Read fills the buffer or terminates.
		_, _ = rand.Read(bytes[:])
		supplied = hex.EncodeToString(bytes[:])
	}
	return context.WithValue(ctx, logger.TraceIDKey, supplied), supplied
}

func valid(value string) bool {
	if len(value) == 0 || len(value) > MaxLength {
		return false
	}
	for i := range len(value) {
		if value[i] < 0x21 || value[i] > 0x7e {
			return false
		}
	}
	return true
}

// FromContext reads the trace identifier used by the logger and outbound clients.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	value, _ := ctx.Value(logger.TraceIDKey).(string)
	return value
}
