package bootstrap

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"olixops-permission-center/internal/infrastructure/cache"
	"olixops-permission-center/internal/infrastructure/persistence"
	"olixops-permission-center/internal/infrastructure/spicedb"
	grpcinterface "olixops-permission-center/internal/interfaces/grpc"
	httpinterface "olixops-permission-center/internal/interfaces/http"
	"olixops-permission-center/pkg/config"
	"olixops-permission-center/pkg/logger"
)

func Run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	appLogger, err := logger.New(cfg.AppEnv)
	if err != nil {
		return err
	}
	logger.Replace(appLogger)
	defer func() { _ = logger.Sync() }()

	database, err := persistence.OpenPostgres(cfg.DatabaseDSN)
	if err != nil {
		return err
	}
	if database != nil {
		pool, err := database.DB()
		if err != nil {
			return errors.New("cannot obtain postgres connection pool")
		}
		defer func() { _ = pool.Close() }()
	}
	redisClient := cache.New(cache.Config{
		Addr:     cfg.RedisAddr,
		Password: cfg.RedisPassword,
		DB:       cfg.RedisDB,
	})
	if redisClient != nil {
		defer func() { _ = redisClient.Close() }()
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := redisClient.Ping(pingCtx).Err()
		cancel()
		if err != nil {
			return errors.New("redis initialization failed")
		}
	}
	spiceClient, err := spicedb.Open(ctx, cfg.SpiceDB)
	if err != nil {
		return err
	}
	if spiceClient != nil {
		defer func() { _ = spiceClient.Close() }()
		logger.Info(ctx, "spicedb connection initialized")
	}

	httpServer := &http.Server{
		Addr: cfg.HTTPAddr, Handler: httpinterface.NewRouter(cfg),
		ReadHeaderTimeout: cfg.HTTPReadTimeout, ReadTimeout: cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout, IdleTimeout: cfg.HTTPIdleTimeout,
	}
	httpListener, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		return errors.New("cannot listen on HTTP address")
	}
	defer func() { _ = httpListener.Close() }()
	grpcServer := grpcinterface.NewServer(cfg)
	grpcListener, err := net.Listen("tcp", cfg.GRPCAddr)
	if err != nil {
		return errors.New("cannot listen on gRPC address")
	}
	defer func() { _ = grpcListener.Close() }()
	defer grpcServer.Stop()
	defer func() { _ = httpServer.Close() }()

	errCh := make(chan error, 2)
	go func() { errCh <- httpServer.Serve(httpListener) }()
	go func() { errCh <- grpcServer.Serve(grpcListener) }()
	logger.Info(ctx, "HTTP and gRPC servers started")

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		stopped := make(chan struct{})
		go func() { grpcServer.GracefulStop(); close(stopped) }()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			_ = httpServer.Close()
		}
		select {
		case <-stopped:
		case <-shutdownCtx.Done():
			grpcServer.Stop()
		}
		return nil
	case runErr := <-errCh:
		if errors.Is(runErr, http.ErrServerClosed) {
			return nil
		}
		return errors.New("server listener stopped unexpectedly")
	}
}
