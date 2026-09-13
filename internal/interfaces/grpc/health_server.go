package grpcinterface

import (
	"context"

	healthv1 "olixops-permission-center/api/gen/health/v1"
	applicationhealth "olixops-permission-center/internal/application/health"
)

type healthServer struct {
	healthv1.UnimplementedHealthServiceServer
	service applicationhealth.Service
}

func newHealthServer() *healthServer {
	return &healthServer{}
}

func (s *healthServer) Healthy(context.Context, *healthv1.HealthyRequest) (*healthv1.HealthyResponse, error) {
	status := s.service.Check()
	return &healthv1.HealthyResponse{
		Healthy: status.Status == "ok",
		Message: status.Status,
	}, nil
}
