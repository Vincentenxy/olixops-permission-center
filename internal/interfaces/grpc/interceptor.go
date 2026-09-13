package grpcinterface

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"olixops-permission-center/pkg/auth"
	"olixops-permission-center/pkg/logger"
	"olixops-permission-center/pkg/requestid"
	"olixops-permission-center/pkg/userctx"
)

func unaryInterceptor(verifier *auth.Verifier, configErr error) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, request any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (result any, err error) {
		ctx = withRequestID(ctx)
		started := time.Now()
		_ = grpc.SetHeader(ctx, metadata.Pairs("x-request-id", traceID(ctx)))
		defer func() {
			if recover() != nil {
				logger.Error(ctx, "grpc panic recovered")
				result, err = nil, status.Error(codes.Internal, "Internal")
			}
			err = sanitizeError(err)
			logger.Info(ctx, "grpc request", zap.String("method", info.FullMethod),
				zap.String("code", status.Code(err).String()), zap.Duration("latency", time.Since(started)))
		}()
		ctx, err = authenticate(ctx, info.FullMethod, verifier, configErr)
		if err != nil {
			return nil, err
		}
		return handler(ctx, request)
	}
}

func streamInterceptor(verifier *auth.Verifier, configErr error) grpc.StreamServerInterceptor {
	return func(server any, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		ctx := withRequestID(stream.Context())
		started := time.Now()
		_ = stream.SetHeader(metadata.Pairs("x-request-id", traceID(ctx)))
		defer func() {
			if recover() != nil {
				logger.Error(ctx, "grpc stream panic recovered")
				err = status.Error(codes.Internal, "Internal")
			}
			err = sanitizeError(err)
			logger.Info(ctx, "grpc stream request", zap.String("method", info.FullMethod),
				zap.String("code", status.Code(err).String()), zap.Duration("latency", time.Since(started)))
		}()
		ctx, err = authenticate(ctx, info.FullMethod, verifier, configErr)
		if err != nil {
			return err
		}
		return handler(server, &contextServerStream{ServerStream: stream, ctx: ctx})
	}
}

func authenticate(ctx context.Context, method string, verifier *auth.Verifier, configErr error) (context.Context, error) {
	switch method {
	case "/health.v1.HealthService/Healthy", "/grpc.health.v1.Health/Check", "/grpc.health.v1.Health/Watch":
		return ctx, nil
	}
	if configErr != nil {
		return ctx, status.Error(codes.Internal, "Internal")
	}
	headers := metadata.ValueFromIncomingContext(ctx, "authorization")
	if len(headers) != 1 {
		return ctx, status.Error(codes.Unauthenticated, "Unauthenticated")
	}
	cookie := ""
	if values := metadata.ValueFromIncomingContext(ctx, "cookie"); len(values) == 1 {
		cookie = values[0]
	}
	user, err := verifier.Authenticate(headers[0], cookie)
	if err != nil {
		return ctx, status.Error(codes.Unauthenticated, "Unauthenticated")
	}
	return userctx.WithContext(ctx, user), nil
}

// Transport adapters map domain failures explicitly. Never expose arbitrary
// upstream error messages, stack traces or status details to an API caller.
func sanitizeError(err error) error {
	if err == nil {
		return nil
	}
	code := status.Code(err)
	switch {
	case errors.Is(err, context.Canceled):
		code = codes.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		code = codes.DeadlineExceeded
	case code == codes.Unknown:
		code = codes.Internal
	}
	return status.Error(code, code.String())
}

func traceID(ctx context.Context) string {
	return requestid.FromContext(ctx)
}

func withRequestID(ctx context.Context) context.Context {
	ctx, _ = requestid.Ensure(ctx, incomingRequestID(ctx))
	return ctx
}

func incomingRequestID(ctx context.Context) string {
	values := metadata.ValueFromIncomingContext(ctx, "x-request-id")
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

type contextServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *contextServerStream) Context() context.Context {
	return s.ctx
}
