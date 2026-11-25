package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
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
		log.Error("api stopped", "err", err)
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
	handler, closeAssets, err := rt.Handler()
	if err != nil {
		return err
	}
	defer closeAssets()
	listener, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, IdleTimeout: 90 * time.Second,
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	result := make(chan error, 1)
	go func() { result <- server.Serve(listener) }()
	log.Info("api listening", "addr", listener.Addr().String())
	select {
	case err := <-result:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			_ = server.Close()
			return err
		}
		return nil
	}
}
