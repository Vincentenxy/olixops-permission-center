// Package spicedb owns the official SpiceDB API clients and connection lifecycle.
// Application/domain code must use domain ports rather than protobuf types.
package spicedb

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"time"

	pb "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"olixops-permission-center/pkg/config"
	"olixops-permission-center/pkg/logger"
)

type Client struct {
	Permissions pb.PermissionsServiceClient
	Schema      pb.SchemaServiceClient
	Watch       pb.WatchServiceClient
	connection  *grpc.ClientConn
	timeout     time.Duration
}

// Open returns nil when explicitly disabled. Enabled clients must pass a bounded
// health check; this checks connectivity, not authorization to business APIs.
func Open(ctx context.Context, cfg config.SpiceDB) (*Client, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if _, _, err := net.SplitHostPort(cfg.Endpoint); err != nil {
		return nil, errors.New("spicedb endpoint must be host:port")
	}
	if strings.TrimSpace(cfg.Token) == "" || strings.ContainsAny(cfg.Token, "\r\n") {
		return nil, errors.New("spicedb token is required and must not contain newlines")
	}
	if cfg.Timeout <= 0 {
		return nil, errors.New("spicedb timeout must be positive")
	}
	transport, err := transportCredentials(cfg)
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(cfg.Endpoint,
		grpc.WithTransportCredentials(transport),
		grpc.WithPerRPCCredentials(bearerCredentials{token: cfg.Token, secure: !cfg.Insecure}),
		grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoke grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			ctx, cancel := context.WithTimeout(outgoingContext(ctx), cfg.Timeout)
			defer cancel()
			return safeRPCError(invoke(ctx, method, req, reply, cc, opts...))
		}),
		grpc.WithStreamInterceptor(func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
			stream, err := streamer(outgoingContext(ctx), desc, cc, method, opts...)
			if err != nil {
				return nil, safeRPCError(err)
			}
			return &safeClientStream{ClientStream: stream}, nil
		}),
	)
	if err != nil {
		return nil, errors.New("cannot initialize spicedb connection")
	}
	client := &Client{
		Permissions: pb.NewPermissionsServiceClient(conn), Schema: pb.NewSchemaServiceClient(conn),
		Watch: pb.NewWatchServiceClient(conn), connection: conn, timeout: cfg.Timeout,
	}
	if err := client.CheckHealth(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return client, nil
}

func (c *Client) CheckHealth(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	result, err := grpc_health_v1.NewHealthClient(c.connection).Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		return safeRPCError(err)
	}
	if result.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		return status.Error(codes.Unavailable, "spicedb is not serving")
	}
	return nil
}

func (c *Client) Close() error {
	if c == nil || c.connection == nil {
		return nil
	}
	return c.connection.Close()
}

func transportCredentials(cfg config.SpiceDB) (credentials.TransportCredentials, error) {
	if cfg.Insecure {
		if cfg.CAFile != "" || cfg.ServerName != "" {
			return nil, errors.New("spicedb TLS options cannot be used with insecure mode")
		}
		return insecure.NewCredentials(), nil
	}
	tlsConfig := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: cfg.ServerName}
	if cfg.CAFile != "" {
		pemData, err := os.ReadFile(cfg.CAFile)
		if err != nil {
			return nil, errors.New("cannot read spicedb CA file")
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(pemData) {
			return nil, errors.New("invalid spicedb CA certificate")
		}
		tlsConfig.RootCAs = pool
	}
	return credentials.NewTLS(tlsConfig), nil
}

type bearerCredentials struct {
	token  string
	secure bool
}

func (b bearerCredentials) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer " + b.token}, nil
}
func (b bearerCredentials) RequireTransportSecurity() bool { return b.secure }

func outgoingContext(ctx context.Context) context.Context {
	if id, ok := ctx.Value(logger.TraceIDKey).(string); ok && id != "" {
		md, _ := metadata.FromOutgoingContext(ctx)
		md = md.Copy()
		md.Set("x-request-id", id)
		return metadata.NewOutgoingContext(ctx, md)
	}
	return ctx
}

func safeRPCError(err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return err
	}
	code := status.Code(err)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		code = status.FromContextError(err).Code()
	}
	return status.Error(code, "spicedb request failed")
}

type safeClientStream struct{ grpc.ClientStream }

func (s *safeClientStream) SendMsg(value any) error {
	return safeRPCError(s.ClientStream.SendMsg(value))
}
func (s *safeClientStream) RecvMsg(value any) error {
	return safeRPCError(s.ClientStream.RecvMsg(value))
}

func (s *safeClientStream) Header() (metadata.MD, error) {
	headers, err := s.ClientStream.Header()
	return headers, safeRPCError(err)
}

func (s *safeClientStream) CloseSend() error { return safeRPCError(s.ClientStream.CloseSend()) }
