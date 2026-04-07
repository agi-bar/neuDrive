package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/agi-bar/agenthub/internal/app/serverapp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := serverapp.Run(ctx, serverapp.Options{}); err != nil {
		slog.Error("server failed", "error", err)
		os.Exit(1)
	}
}
