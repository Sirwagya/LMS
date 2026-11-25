package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"vedam/lms/internal/runtime"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, log); err != nil {
		log.Error("worker stopped", "err", err)
		os.Exit(1)
	}
}
func run(ctx context.Context, log *slog.Logger) error {
	cfg, err := runtime.LoadConfig()
	if err != nil {
		return err
	}
	start, cancel := context.WithTimeout(ctx, 30*time.Second)
	rt, err := runtime.Open(start, cfg, log)
	cancel()
	if err != nil {
		return err
	}
	defer rt.Close()
	log.Info("worker running")
	rt.Work(ctx)
	return nil
}
