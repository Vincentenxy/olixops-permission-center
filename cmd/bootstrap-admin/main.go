// bootstrap-admin creates the first administrator explicitly; it is never run
// by the HTTP server and never echoes the supplied password or credentials.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	management "olixops-permission-center/internal/application/management"
	"olixops-permission-center/internal/infrastructure/persistence"
	"olixops-permission-center/pkg/config"
	"olixops-permission-center/pkg/logger"
)

func main() {
	if run() != nil {
		logger.L().Error("administrator initialization failed; check configuration, input policy and empty database requirement")
		os.Exit(1)
	}
}
func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	l, err := logger.New(cfg.AppEnv)
	if err != nil {
		return err
	}
	logger.Replace(l)
	defer func() { _ = logger.Sync() }()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	db, err := persistence.OpenPostgres(cfg.DatabaseDSN)
	if err != nil {
		return err
	}
	if db != nil {
		pool, e := db.DB()
		if e != nil {
			return e
		}
		defer func() { _ = pool.Close() }()
	}
	service := management.New(persistence.NewManagementStore(db), nil, cfg.AuthJWTIssuer)
	_, err = service.Bootstrap(ctx, management.CreateUser{Username: os.Getenv("BOOTSTRAP_USERNAME"), Password: os.Getenv("BOOTSTRAP_PASSWORD"), Name: os.Getenv("BOOTSTRAP_NAME")})
	if err == nil {
		logger.Info(ctx, "initial platform administrator created")
	}
	return err
}
