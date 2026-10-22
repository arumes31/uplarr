package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"uplarr/internal/api"
	"uplarr/internal/logger"
	"uplarr/internal/models"
	"uplarr/internal/queue"
	"uplarr/internal/sftpclient"
)

const authPasswordFile = ".auth_password"

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

// resolveAuthPassword returns the Web UI password. Running without one must be
// requested explicitly with ALLOW_NO_AUTH=true; otherwise a random password is
// generated on first start, stored in the config directory and logged once.
func resolveAuthPassword(configDir string) (string, error) {
	if pw := getEnv("AUTH_PASSWORD", ""); pw != "" {
		return pw, nil
	}
	if os.Getenv("ALLOW_NO_AUTH") == "true" {
		logger.Warn("AUTH_PASSWORD is empty and ALLOW_NO_AUTH=true: the Web UI is open to anyone who can reach it")
		return "", nil
	}

	if err := os.MkdirAll(configDir, 0750); err != nil {
		return "", fmt.Errorf("create config directory: %w", err)
	}
	root, err := os.OpenRoot(configDir)
	if err != nil {
		return "", fmt.Errorf("open config directory: %w", err)
	}
	defer func() { _ = root.Close() }()

	data, err := root.ReadFile(authPasswordFile)
	if err == nil && strings.TrimSpace(string(data)) != "" {
		logger.Info(fmt.Sprintf("AUTH_PASSWORD not set; using the generated password stored in %s", filepath.Join(configDir, authPasswordFile)))
		return strings.TrimSpace(string(data)), nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("read %s: %w", authPasswordFile, err)
	}

	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	pw := base64.RawURLEncoding.EncodeToString(b)
	if err := root.WriteFile(authPasswordFile, []byte(pw+"\n"), 0600); err != nil {
		return "", fmt.Errorf("store generated password: %w", err)
	}
	logger.Warn(fmt.Sprintf("AUTH_PASSWORD not set; generated a Web UI password and stored it in %s: %s", filepath.Join(configDir, authPasswordFile), pw))
	return pw, nil
}

func Run() error {
	ctx, stop := newRunContext()
	defer stop()

	config := models.Config{
		LocalDir:   getEnv("LOCAL_DIR", "./test_data"),
		ConfigDir:  getEnv("CONFIG_DIR", "./config"),
		WebPort:    getEnv("WEB_PORT", "8080"),
		TrustProxy: os.Getenv("TRUST_PROXY") == "true",
	}
	authPassword, err := resolveAuthPassword(config.ConfigDir)
	if err != nil {
		return fmt.Errorf("auth setup failed: %w", err)
	}
	config.AuthPassword = authPassword
	bindAddr := getEnv("BIND_ADDR", "127.0.0.1")

	sftpclient.SetKnownHostsFile(
		getEnv("SSH_KNOWN_HOSTS", filepath.Join(config.ConfigDir, "known_hosts")),
		getEnv("SSH_TOFU", "true") == "true",
	)
	sftpclient.SetKeysDir(getEnv("SSH_KEYS_DIR", filepath.Join(config.ConfigDir, "keys")))

	qm := queue.NewQueueManager(config.LocalDir, config.ConfigDir)
	defer qm.Shutdown()

	mux, err := apiSetupApp(config, qm)
	if err != nil {
		return fmt.Errorf("setup failed: %w", err)
	}

	addr := net.JoinHostPort(bindAddr, config.WebPort)
	logger.Info(fmt.Sprintf("Server starting on %s", addr))
	server := &http.Server{
		Addr:              addr,
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
