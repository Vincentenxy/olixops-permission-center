package logger

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// TraceIDKey is used by inbound adapters to attach the request trace id.
var TraceIDKey traceIDKey

type traceIDKey struct{}

var (
	mu     sync.RWMutex
	global = zap.NewNop()
)

func New(environment string) (*zap.Logger, error) {
	development := environment != "production" && environment != "release"
	if development {
		cfg := zap.NewDevelopmentConfig()
		cfg.Encoding = "console"
		cfg.EncoderConfig.EncodeTime = encodeTime
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
		cfg.OutputPaths = []string{"stdout"}
		return cfg.Build()
	}

	cfg := zap.NewProductionConfig()
	cfg.EncoderConfig.EncodeTime = encodeTime
	cfg.OutputPaths = []string{"stdout"}
	return cfg.Build()
}

func Replace(next *zap.Logger) {
	if next == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	global = next
}

func L() *zap.Logger {
	mu.RLock()
	defer mu.RUnlock()
	return global
}

func Info(ctx context.Context, message string, fields ...zap.Field) {
	L().Info(message, withTrace(ctx, fields)...)
}

func Error(ctx context.Context, message string, fields ...zap.Field) {
	L().Error(message, withTrace(ctx, fields)...)
}

func ErrorField(err error) zap.Field {
	return zap.Error(err)
}

func Sync() error {
	return L().Sync()
}

func withTrace(ctx context.Context, fields []zap.Field) []zap.Field {
	if ctx == nil {
		return fields
	}
	traceID, ok := ctx.Value(TraceIDKey).(string)
	if !ok || traceID == "" {
		return fields
	}
	return append(fields, zap.String("trace-id", traceID))
}

func encodeTime(value time.Time, encoder zapcore.PrimitiveArrayEncoder) {
	encoder.AppendString(value.Format("2006-01-02T15:04:05.000-07:00"))
}
