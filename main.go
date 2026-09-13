package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"olixops-permission-center/internal/bootstrap"
	"olixops-permission-center/pkg/logger"
)

// @title OlixOps Permission Center
// @version 0.1.0
// @description Independent authorization center bootstrap APIs.
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
func main() {
	// Install a safe logger before configuration loading can fail.
	if initial, err := logger.New("production"); err == nil {
		logger.Replace(initial)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := bootstrap.Run(ctx); err != nil {
		logger.L().Error("server stopped", logger.ErrorField(err))
		_ = logger.Sync()
		os.Exit(1)
	}
}
