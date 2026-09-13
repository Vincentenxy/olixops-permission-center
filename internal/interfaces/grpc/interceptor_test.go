package grpcinterface

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"olixops-permission-center/pkg/auth"
	"olixops-permission-center/pkg/userctx"
)

func interceptorFixture(t *testing.T) (*auth.Verifier, context.Context) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	verifier, err := auth.NewVerifier(base64.StdEncoding.EncodeToString(der), "test-issuer", "test-audience")
	if err != nil {
		t.Fatal(err)
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{"iss": "test-issuer", "aud": "test-audience", "exp": time.Now().Add(time.Hour).Unix(), "sub": "test-user"}).SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return verifier, metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer "+token, "x-request-id", "test-trace"))
}

func TestUnaryAuthenticationAndRecovery(t *testing.T) {
	verifier, authenticated := interceptorFixture(t)
	interceptor := unaryInterceptor(verifier, nil)
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Permission/Check"}
	called := false
	handler := func(ctx context.Context, request any) (any, error) {
		called = true
		user, ok := userctx.FromContext(ctx)
		if !ok || user.UserID != "test-user" || traceID(ctx) != "test-trace" {
			t.Fatal("identity or trace missing")
		}
		return "allowed", nil
	}
	for _, ctx := range []context.Context{context.Background(), metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer fake-token")), metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", "Bearer one", "authorization", "Bearer two"))} {
		_, err := interceptor(ctx, nil, info, handler)
		if status.Code(err) != codes.Unauthenticated || called {
			t.Fatal("unauthenticated handler executed")
		}
	}
	result, err := interceptor(authenticated, nil, info, handler)
	if err != nil || result != "allowed" || !called {
		t.Fatal("authenticated handler failed")
	}
	for _, tc := range []struct {
		name    string
		handler grpc.UnaryHandler
		code    codes.Code
	}{
		{"panic", func(context.Context, any) (any, error) { panic("fake-sensitive-panic") }, codes.Internal},
		{"internal error", func(context.Context, any) (any, error) { return nil, errors.New("fake-sensitive-connection") }, codes.Internal},
		{"permission denied", func(context.Context, any) (any, error) {
			return nil, status.Error(codes.PermissionDenied, "fake-sensitive-denial")
		}, codes.PermissionDenied},
		{"deadline", func(context.Context, any) (any, error) { return nil, context.DeadlineExceeded }, codes.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := interceptor(authenticated, nil, info, tc.handler)
			if status.Code(err) != tc.code || status.Convert(err).Message() != tc.code.String() {
				t.Fatalf("error was not sanitized: %v", err)
			}
		})
	}
	_, err = unaryInterceptor(nil, errors.New("invalid config"))(context.Background(), nil, info, handler)
	if status.Code(err) != codes.Internal {
		t.Fatal("missing configuration did not fail closed")
	}
}

type testServerStream struct {
	grpc.ServerStream
	ctx     context.Context
	headers metadata.MD
}

func (s *testServerStream) Context() context.Context       { return s.ctx }
func (s *testServerStream) SetHeader(md metadata.MD) error { s.headers = md; return nil }

func TestStreamAuthenticationTraceAndRecovery(t *testing.T) {
	verifier, authenticated := interceptorFixture(t)
	interceptor := streamInterceptor(verifier, nil)
	info := &grpc.StreamServerInfo{FullMethod: "/test.Permission/Watch", IsServerStream: true}
	called := false
	handler := func(_ any, stream grpc.ServerStream) error {
		called = true
		user, ok := userctx.FromContext(stream.Context())
		if !ok || user.UserID != "test-user" || traceID(stream.Context()) != "test-trace" {
			t.Fatal("stream identity or trace missing")
		}
		return nil
	}
	err := interceptor(nil, &testServerStream{ctx: context.Background()}, info, handler)
	if status.Code(err) != codes.Unauthenticated || called {
		t.Fatal("unauthenticated stream handler executed")
	}
	stream := &testServerStream{ctx: authenticated}
	if err := interceptor(nil, stream, info, handler); err != nil || !called {
		t.Fatal("authenticated stream failed")
	}
	if values := stream.headers.Get("x-request-id"); len(values) != 1 || values[0] != "test-trace" {
		t.Fatal("trace header was not propagated")
	}
	err = interceptor(nil, stream, info, func(any, grpc.ServerStream) error { panic("fake-sensitive-stream") })
	if status.Code(err) != codes.Internal || status.Convert(err).Message() != "Internal" {
		t.Fatal("stream panic not sanitized")
	}
	err = interceptor(nil, stream, info, func(any, grpc.ServerStream) error { return status.Error(codes.Unavailable, "fake-sensitive-upstream") })
	if status.Code(err) != codes.Unavailable || status.Convert(err).Message() != "Unavailable" {
		t.Fatal("stream error not sanitized")
	}
}

func TestPublicHealthAllowlistIsExact(t *testing.T) {
	for _, method := range []string{"/health.v1.HealthService/Healthy", "/grpc.health.v1.Health/Check", "/grpc.health.v1.Health/Watch"} {
		if _, err := authenticate(context.Background(), method, nil, errors.New("not configured")); err != nil {
			t.Fatalf("public health %s rejected", method)
		}
	}
	for _, method := range []string{"/test.Health/Check", "/health.v1.HealthService/HealthyPrivate", "/grpc.health.v1.Health/List"} {
		if _, err := authenticate(context.Background(), method, nil, nil); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("non-public method %s bypassed authentication", method)
		}
	}
}
