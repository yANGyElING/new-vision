package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/new-vision-lab/new-vision/internal/nodeapp"
	"github.com/new-vision-lab/new-vision/internal/platform"
)

var version = "dev"

func main() {
	cfg, err := nodeapp.LoadConfig(os.LookupEnv)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(2)
	}
	logger := platform.NewLogger(cfg.HTTP.LogLevel)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	app, err := nodeapp.New(ctx, cfg, version, logger)
	if err != nil {
		logger.Error("application initialization failed", "error", err)
		os.Exit(1)
	}
	defer app.Close()

	server := platform.NewServer(cfg.HTTP.Addr, app.Handler)
	if err := platform.Serve(ctx, server, cfg.HTTP.ShutdownTimeout, logger); err != nil {
		logger.Error("http server failed", "error", err)
		os.Exit(1)
	}
}
