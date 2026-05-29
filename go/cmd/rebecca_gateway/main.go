package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rebeccapanel/rebecca/go/internal/gateway"
)

func main() {
	cfg := gateway.LoadConfig()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	python, err := gateway.StartPython(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to start python runtime: %v", err)
	}
	defer python.Stop()

	server, err := gateway.NewServer(cfg)
	if err != nil {
		log.Fatalf("failed to initialize gateway: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("rebecca gateway listening on %s and proxying python on %s", cfg.Addr, cfg.PythonAddr())
		errCh <- server.Run()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("gateway shutdown failed: %v", err)
		}
	case err := <-errCh:
		if err != nil {
			log.Fatalf("gateway failed: %v", err)
		}
	}
}
