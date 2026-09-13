package spicedb

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"olixops-permission-center/pkg/config"
	"olixops-permission-center/pkg/logger"
)

type fakeSpiceDB struct {
	grpc_health_v1.UnimplementedHealthServer
	pb.UnimplementedPermissionsServiceServer
	health     func(context.Context) (*grpc_health_v1.HealthCheckResponse, error)
	permission func(context.Context, *pb.CheckPermissionRequest) (*pb.CheckPermissionResponse, error)
	watch      func(grpc.ServerStreamingServer[pb.WatchResponse]) error
}

type fakeWatch struct {
	pb.UnimplementedWatchServiceServer
	watch func(grpc.ServerStreamingServer[pb.WatchResponse]) error
}

func (s *fakeWatch) Watch(_ *pb.WatchRequest, stream grpc.ServerStreamingServer[pb.WatchResponse]) error {
	if s.watch != nil {
		return s.watch(stream)
	}
	return nil
}

func (s *fakeSpiceDB) Check(ctx context.Context, _ *grpc_health_v1.HealthCheckRequest) (*grpc_health_v1.HealthCheckResponse, error) {
	if s.health != nil {
		return s.health(ctx)
	}
	return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
}

func (s *fakeSpiceDB) CheckPermission(ctx context.Context, request *pb.CheckPermissionRequest) (*pb.CheckPermissionResponse, error) {
	if s.permission != nil {
		return s.permission(ctx, request)
	}
	return &pb.CheckPermissionResponse{Permissionship: pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION}, nil
}

func startFakeSpiceDB(t *testing.T, service *fakeSpiceDB, options ...grpc.ServerOption) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer(options...)
	grpc_health_v1.RegisterHealthServer(server, service)
	pb.RegisterPermissionsServiceServer(server, service)
	pb.RegisterWatchServiceServer(server, &fakeWatch{watch: service.watch})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })
	return listener.Addr().String()
}

func testConfig(endpoint string) config.SpiceDB {
	return config.SpiceDB{Enabled: true, Endpoint: endpoint, Token: "fake-spicedb-token", Insecure: true, Timeout: 2 * time.Second}
}

func assertSafeError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), "fake-sensitive") || strings.Contains(err.Error(), "fake-spicedb-token") {
		t.Fatal("upstream details or credentials escaped in error")
	}
}

func TestOpenConfigurationAndDisabled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, err := Open(ctx, config.SpiceDB{Endpoint: "invalid", Token: "fake-sensitive-token"})
	if err != nil || client != nil {
		t.Fatal("disabled integration attempted initialization")
	}
	if err := client.Close(); err != nil {
		t.Fatal("nil client close failed")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*config.SpiceDB)
	}{
		{"invalid endpoint", func(c *config.SpiceDB) { c.Endpoint = "invalid-endpoint" }},
		{"empty token", func(c *config.SpiceDB) { c.Token = " " }},
		{"newline token", func(c *config.SpiceDB) { c.Token = "fake-sensitive\ntoken" }},
		{"zero timeout", func(c *config.SpiceDB) { c.Timeout = 0 }},
		{"negative timeout", func(c *config.SpiceDB) { c.Timeout = -time.Second }},
		{"insecure with CA", func(c *config.SpiceDB) { c.CAFile = "fake-sensitive-ca.pem" }},
		{"insecure with server name", func(c *config.SpiceDB) { c.ServerName = "fake-sensitive-name" }},
		{"missing CA file", func(c *config.SpiceDB) {
			c.Insecure = false
			c.CAFile = filepath.Join(t.TempDir(), "fake-sensitive-missing.pem")
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig("127.0.0.1:1")
			tc.mutate(&cfg)
			client, err := Open(context.Background(), cfg)
			assertSafeError(t, err)
			if client != nil {
				t.Fatal("invalid configuration returned a client")
			}
		})
	}
}

func TestGeneratedClientsMetadataAndClose(t *testing.T) {
	type observation struct {
		auth, trace, existing string
		deadline              bool
		permission            string
	}
	observed := make(chan observation, 2)
	observe := func(ctx context.Context, permission string) {
		md, _ := metadata.FromIncomingContext(ctx)
		_, hasDeadline := ctx.Deadline()
		observed <- observation{strings.Join(md.Get("authorization"), ","), strings.Join(md.Get("x-request-id"), ","), strings.Join(md.Get("test-existing"), ","), hasDeadline, permission}
	}
	endpoint := startFakeSpiceDB(t, &fakeSpiceDB{
		health: func(ctx context.Context) (*grpc_health_v1.HealthCheckResponse, error) {
			observe(ctx, "health")
			return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_SERVING}, nil
		},
		permission: func(ctx context.Context, request *pb.CheckPermissionRequest) (*pb.CheckPermissionResponse, error) {
			observe(ctx, request.GetPermission())
			if request.GetResource().GetObjectId() != "document-1" || request.GetSubject().GetObject().GetObjectId() != "user-1" {
				return nil, status.Error(codes.InvalidArgument, "unexpected test request")
			}
			return &pb.CheckPermissionResponse{Permissionship: pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION}, nil
		},
	})
	ctx := context.WithValue(context.Background(), logger.TraceIDKey, "test-trace")
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("test-existing", "preserved"))
	client, err := Open(ctx, testConfig(endpoint))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if client.Permissions == nil || client.Schema == nil || client.Watch == nil {
		t.Fatal("official generated API clients missing")
	}
	response, err := client.Permissions.CheckPermission(ctx, &pb.CheckPermissionRequest{
		Resource:   &pb.ObjectReference{ObjectType: "test/document", ObjectId: "document-1"},
		Permission: "view", Subject: &pb.SubjectReference{Object: &pb.ObjectReference{ObjectType: "test/user", ObjectId: "user-1"}},
	})
	if err != nil || response.GetPermissionship() != pb.CheckPermissionResponse_PERMISSIONSHIP_HAS_PERMISSION {
		t.Fatalf("generated CheckPermission call failed: %v", err)
	}
	for _, permission := range []string{"health", "view"} {
		select {
		case got := <-observed:
			if got.auth != "Bearer fake-spicedb-token" || got.trace != "test-trace" || got.existing != "preserved" || !got.deadline || got.permission != permission {
				t.Fatal("metadata, deadline, or RPC request was not propagated")
			}
		case <-time.After(time.Second):
			t.Fatal("server did not observe expected RPC")
		}
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = client.Permissions.CheckPermission(context.Background(), &pb.CheckPermissionRequest{})
	if err == nil {
		t.Fatal("closed client continued to make RPCs")
	}
}

func TestHealthFailureAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		check  func(context.Context) (*grpc_health_v1.HealthCheckResponse, error)
		cancel bool
		code   codes.Code
	}{
		{"not serving", func(context.Context) (*grpc_health_v1.HealthCheckResponse, error) {
			return &grpc_health_v1.HealthCheckResponse{Status: grpc_health_v1.HealthCheckResponse_NOT_SERVING}, nil
		}, false, codes.Unavailable},
		{"upstream error", func(context.Context) (*grpc_health_v1.HealthCheckResponse, error) {
			return nil, status.Error(codes.Internal, "fake-sensitive-database")
		}, false, codes.Internal},
		{"deadline", func(ctx context.Context) (*grpc_health_v1.HealthCheckResponse, error) {
			<-ctx.Done()
			return nil, status.Error(codes.DeadlineExceeded, "fake-sensitive-timeout")
		}, false, codes.DeadlineExceeded},
		{"caller canceled", nil, true, codes.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoint := startFakeSpiceDB(t, &fakeSpiceDB{health: tc.check})
			cfg := testConfig(endpoint)
			cfg.Timeout = 250 * time.Millisecond
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			started := time.Now()
			client, err := Open(ctx, cfg)
			assertSafeError(t, err)
			if status.Code(err) != tc.code {
				t.Fatalf("health failure lost its status code: %v", err)
			}
			if client != nil {
				t.Fatal("unhealthy dependency returned a usable client")
			}
			if time.Since(started) > 2*time.Second {
				t.Fatal("health initialization exceeded bounded deadline")
			}
		})
	}
}

func TestWatchStreamMetadataAndErrorRedaction(t *testing.T) {
	for _, tc := range []struct {
		name  string
		final error
	}{
		{"server error", status.Error(codes.Unavailable, "fake-sensitive-watch-error")},
		{"normal end", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			observed := make(chan bool, 1)
			endpoint := startFakeSpiceDB(t, &fakeSpiceDB{watch: func(stream grpc.ServerStreamingServer[pb.WatchResponse]) error {
				md, _ := metadata.FromIncomingContext(stream.Context())
				observed <- strings.Join(md.Get("authorization"), ",") == "Bearer fake-spicedb-token" && strings.Join(md.Get("x-request-id"), ",") == "watch-trace"
				return tc.final
			}})
			client, err := Open(context.Background(), testConfig(endpoint))
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), logger.TraceIDKey, "watch-trace"), time.Second)
			defer cancel()
			stream, err := client.Watch.Watch(ctx, &pb.WatchRequest{})
			if err != nil {
				t.Fatal(err)
			}
			_, err = stream.Recv()
			if tc.final == nil {
				if !errors.Is(err, io.EOF) {
					t.Fatalf("normal stream end changed: %v", err)
				}
			} else {
				assertSafeError(t, err)
				if status.Code(err) != codes.Unavailable {
					t.Fatal("stream error status lost")
				}
			}
			select {
			case ok := <-observed:
				if !ok {
					t.Fatal("stream credentials or trace missing")
				}
			case <-ctx.Done():
				t.Fatal("stream did not reach fake server")
			}
		})
	}
}

func TestPermissionTimeoutCancellationAndErrorRedaction(t *testing.T) {
	for _, tc := range []struct {
		name   string
		check  func(context.Context, *pb.CheckPermissionRequest) (*pb.CheckPermissionResponse, error)
		cancel bool
		code   codes.Code
	}{
		{"upstream error", func(context.Context, *pb.CheckPermissionRequest) (*pb.CheckPermissionResponse, error) {
			return nil, status.Error(codes.PermissionDenied, "fake-sensitive-authorization-detail")
		}, false, codes.PermissionDenied},
		{"deadline", func(ctx context.Context, _ *pb.CheckPermissionRequest) (*pb.CheckPermissionResponse, error) {
			<-ctx.Done()
			return nil, status.Error(codes.DeadlineExceeded, "fake-sensitive-deadline")
		}, false, codes.DeadlineExceeded},
		{"caller cancellation", nil, true, codes.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			endpoint := startFakeSpiceDB(t, &fakeSpiceDB{permission: tc.check})
			cfg := testConfig(endpoint)
			cfg.Timeout = 250 * time.Millisecond
			client, err := Open(context.Background(), cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancel {
				cancel()
			}
			started := time.Now()
			_, err = client.Permissions.CheckPermission(ctx, &pb.CheckPermissionRequest{})
			assertSafeError(t, err)
			if status.Code(err) != tc.code {
				t.Fatalf("wrong error classification: %v", err)
			}
			if time.Since(started) > 2*time.Second {
				t.Fatal("permission RPC exceeded deadline")
			}
		})
	}
}

func testTLSCertificate(t *testing.T) (tls.Certificate, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "spicedb.test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		DNSNames: []string{"spicedb.test"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		IsCA: true, BasicConstraintsValid: true,
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	cert, err := tls.X509KeyPair(certPEM, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	if err != nil {
		t.Fatal(err)
	}
	caPath := filepath.Join(t.TempDir(), "test-ca.pem")
	if err := os.WriteFile(caPath, certPEM, 0600); err != nil {
		t.Fatal(err)
	}
	return cert, caPath
}

func TestTLSVerification(t *testing.T) {
	certificate, caPath := testTLSCertificate(t)
	endpoint := startFakeSpiceDB(t, &fakeSpiceDB{}, grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12})))
	cfg := testConfig(endpoint)
	cfg.Insecure = false
	cfg.CAFile, cfg.ServerName = caPath, "spicedb.test"
	client, err := Open(context.Background(), cfg)
	if err != nil {
		t.Fatalf("trusted TLS endpoint rejected: %v", err)
	}
	_ = client.Close()
	_, wrongCA := testTLSCertificate(t)
	for _, tc := range []struct {
		name   string
		mutate func(*config.SpiceDB)
	}{
		{"untrusted certificate", func(c *config.SpiceDB) { c.CAFile = wrongCA }},
		{"wrong server name", func(c *config.SpiceDB) { c.ServerName = "other.test" }},
		{"TLS against plaintext", func(c *config.SpiceDB) {
			c.Endpoint = startFakeSpiceDB(t, &fakeSpiceDB{})
			c.CAFile = ""
			c.ServerName = ""
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := cfg
			candidate.Timeout = 750 * time.Millisecond
			tc.mutate(&candidate)
			client, err := Open(context.Background(), candidate)
			assertSafeError(t, err)
			if client != nil {
				_ = client.Close()
				t.Fatal("invalid TLS connection accepted")
			}
		})
	}
}
