package grpcinterface

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	healthv1 "olixops-permission-center/api/gen/health/v1"
	"olixops-permission-center/pkg/auth"
	"olixops-permission-center/pkg/config"
)

func NewServer(cfg config.Config) *grpc.Server {
	verifier, configErr := auth.NewVerifier(cfg.AuthJWTPublicKey, cfg.AuthJWTIssuer, cfg.AuthJWTAudience)
	server := grpc.NewServer(
		grpc.UnaryInterceptor(unaryInterceptor(verifier, configErr)),
		grpc.StreamInterceptor(streamInterceptor(verifier, configErr)),
	)
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	grpc_health_v1.RegisterHealthServer(server, healthServer)
	healthv1.RegisterHealthServiceServer(server, newHealthServer())
	return server
}
