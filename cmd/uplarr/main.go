package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"uplarr/internal/api"
	"uplarr/internal/logger"
	"uplarr/internal/models"
	"uplarr/internal/queue"
)

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		i, err := strconv.Atoi(value)
		if err != nil {
			return fallback
		}
		return i
	}
	return fallback
}

var (
	apiSetupApp  = api.SetupApp
	httpServe    = func(server *http.Server) error { return server.ListenAndServe() }
	httpShutdown = func(server *http.Server, ctx context.Context) error {
		return server.Shutdown(ctx)
	}
	newRunContext = func() (context.Context, context.CancelFunc) {
		return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	}
	osExit = os.Exit
)

func runHTTPServer(ctx context.Context, server *http.Server) error {
	serveErr := make(chan error, 1)
	go func() {
		serveErr <- httpServe(server)
	}()

	select {
	case err := <-serveErr:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := httpShutdown(server, shutdownCtx); err != nil {
			return fmt.Errorf("shut down HTTP server: %w", err)
		}
		err := <-serveErr
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP during shutdown: %w", err)
		}
		return nil
	}
}

func Run() error {
	ctx, stop := newRunContext()
	defer stop()

	config := models.Config{
		LocalDir:     getEnv("LOCAL_DIR", "./test_data"),
		ConfigDir:    getEnv("CONFIG_DIR", "./config"),
		WebPort:      getEnv("WEB_PORT", "8080"),
		AuthPassword: getEnv("AUTH_PASSWORD", ""),
		TrustProxy:   os.Getenv("TRUST_PROXY") == "true",
	}

	qm := queue.NewQueueManager(config.LocalDir, config.ConfigDir)
	defer qm.Shutdown()

	mux, err := apiSetupApp(config, qm)
	if err != nil {
		return fmt.Errorf("setup failed: %w", err)
	}

	logger.Info(fmt.Sprintf("Server starting on port: %s (binding to 0.0.0.0)", config.WebPort))
	server := &http.Server{
		Addr:              "0.0.0.0:" + config.WebPort,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    1 << 20,
	}
	return runHTTPServer(ctx, server)
}

func main() {
	if err := Run(); err != nil {
		logger.Error(fmt.Sprintf("Application failed: %v", err))
		osExit(1)
	}
}
