package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/new-vision-lab/new-vision/internal/nodeaccess"
	"github.com/new-vision-lab/new-vision/internal/platform"
)

var version = "dev"

func main() {
	cfg, err := platform.LoadHTTPConfig(os.LookupEnv)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(2)
	}
	logger := platform.NewLogger(cfg.LogLevel)

	handler, err := nodeaccess.NewHandler(version)
	if err != nil {
		logger.Error("application initialization failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	server := platform.NewServer(cfg.Addr, handler)
	if err := platform.Serve(ctx, server, cfg.ShutdownTimeout, logger); err != nil {
		logger.Error("http server failed", "error", err)
		os.Exit(1)
	}
}
