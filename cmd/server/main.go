package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"olixops-permission-center/internal/bootstrap"
	"olixops-permission-center/pkg/logger"
)

func main() {
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
