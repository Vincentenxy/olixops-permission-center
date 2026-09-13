package grpcinterface

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	healthv1 "olixops-permission-center/api/gen/health/v1"
	"olixops-permission-center/pkg/config"
)

func TestHealthy(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for gRPC: %v", err)
	}
	server := NewServer(config.Config{})
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- server.Serve(listener)
	}()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	connection, err := grpc.NewClient(
		listener.Addr().String(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("create gRPC client: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })

	var headers metadata.MD
	response, err := healthv1.NewHealthServiceClient(connection).Healthy(ctx, &healthv1.HealthyRequest{}, grpc.Header(&headers))
	if err != nil {
		t.Fatalf("call Healthy: %v", err)
	}
	if !response.GetHealthy() {
		t.Fatalf("Healthy returned false: %q", response.GetMessage())
	}
	if response.GetMessage() != "ok" {
		t.Fatalf("unexpected Healthy message: %q", response.GetMessage())
	}
	if len(headers.Get("x-request-id")) != 1 || headers.Get("x-request-id")[0] == "" {
		t.Fatal("healthy response did not include a trace ID")
	}
	client := grpc_health_v1.NewHealthClient(connection)
	standard, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil || standard.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("standard health failed: %v", err)
	}
	stream, err := client.Watch(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		t.Fatalf("watch health failed: %v", err)
	}
	initial, err := stream.Recv()
	if err != nil || initial.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		t.Fatalf("watch initial health failed: %v", err)
	}

	select {
	case err := <-serveErr:
		if err != nil {
			t.Fatalf("serve gRPC: %v", err)
		}
	default:
	}
}
