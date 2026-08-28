package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"uplarr/internal/models"
	"uplarr/internal/queue"
)

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	t.Setenv(key, "")
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset %s: %v", key, err)
	}
}

func stubRunContext(t *testing.T) context.CancelFunc {
	t.Helper()
	t.Setenv("CONFIG_DIR", t.TempDir())
	t.Setenv("LOCAL_DIR", t.TempDir())
	oldNewRunContext := newRunContext
	ctx, cancel := context.WithCancel(context.Background())
	newRunContext = func() (context.Context, context.CancelFunc) {
		return ctx, func() {}
	}
	t.Cleanup(func() {
		cancel()
		newRunContext = oldNewRunContext
	})
	return cancel
}

func stubHTTPFunctions(t *testing.T) {
	t.Helper()
	oldServe := httpServe
	oldShutdown := httpShutdown
	t.Cleanup(func() {
		httpServe = oldServe
		httpShutdown = oldShutdown
	})
}

func TestGetEnv(t *testing.T) {
	const missingKey = "UPLARR_TEST_GET_ENV_MISSING"

	t.Setenv("TEST_VAR", "value")
	unsetEnv(t, missingKey)
	if getEnv("TEST_VAR", "fallback") != "value" {
		t.Error("Expected value")
	}
	if getEnv(missingKey, "fallback") != "fallback" {
		t.Error("Expected fallback")
	}
}

func TestGetEnvInt(t *testing.T) {
	const missingKey = "UPLARR_TEST_GET_ENV_INT_MISSING"

	t.Setenv("TEST_INT", "123")
	unsetEnv(t, missingKey)
	if getEnvInt("TEST_INT", 0) != 123 {
		t.Error("Expected 123")
	}
	if getEnvInt(missingKey, 456) != 456 {
		t.Error("Expected 456")
	}
	t.Setenv("INVALID_INT", "abc")
	if getEnvInt("INVALID_INT", 789) != 789 {
		t.Error("Expected fallback for invalid int")
	}
}

func TestRunSuccess(t *testing.T) {
	oldSetup := apiSetupApp
	t.Cleanup(func() { apiSetupApp = oldSetup })
	stubRunContext(t)
	stubHTTPFunctions(t)

	apiSetupApp = func(config models.Config, qm *queue.QueueManager) (*http.ServeMux, error) {
		return http.NewServeMux(), nil
	}
	var server *http.Server
	httpServe = func(got *http.Server) error {
		server = got
		return nil
	}

	if err := Run(); err != nil {
		t.Errorf("Expected success, got %v", err)
	}
	if server == nil {
		t.Fatal("expected HTTP server to be created")
	}
	if server.ReadHeaderTimeout != 10*time.Second || server.ReadTimeout != 30*time.Second ||
		server.WriteTimeout != 30*time.Second || server.IdleTimeout != 2*time.Minute {
		t.Fatalf("unexpected HTTP timeouts: %+v", server)
	}
	if server.MaxHeaderBytes != 1<<20 {
		t.Fatalf("unexpected MaxHeaderBytes: %d", server.MaxHeaderBytes)
	}
}

func TestRunSetupFailure(t *testing.T) {
	oldSetup := apiSetupApp
	t.Cleanup(func() { apiSetupApp = oldSetup })
	stubRunContext(t)

	apiSetupApp = func(config models.Config, qm *queue.QueueManager) (*http.ServeMux, error) {
		return nil, fmt.Errorf("setup fail")
	}

	if err := Run(); err == nil {
		t.Error("Expected setup failure")
	}
}

func TestRunGracefulShutdown(t *testing.T) {
	oldSetup := apiSetupApp
	t.Cleanup(func() { apiSetupApp = oldSetup })
	cancel := stubRunContext(t)
	stubHTTPFunctions(t)

	apiSetupApp = func(config models.Config, qm *queue.QueueManager) (*http.ServeMux, error) {
		return http.NewServeMux(), nil
	}
	started := make(chan struct{})
	stopped := make(chan struct{})
	httpServe = func(*http.Server) error {
		close(started)
		<-stopped
		return http.ErrServerClosed
	}
	shutdownCalled := false
	httpShutdown = func(_ *http.Server, ctx context.Context) error {
		shutdownCalled = true
		if _, ok := ctx.Deadline(); !ok {
			t.Error("shutdown context must have a deadline")
		}
		close(stopped)
		return nil
	}

	done := make(chan error, 1)
	go func() { done <- Run() }()
	<-started
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run() after signal = %v", err)
	}
	if !shutdownCalled {
		t.Fatal("expected graceful HTTP shutdown")
	}
}

func TestRunShutdownFailure(t *testing.T) {
	oldSetup := apiSetupApp
	t.Cleanup(func() { apiSetupApp = oldSetup })
	cancel := stubRunContext(t)
	stubHTTPFunctions(t)

	apiSetupApp = func(config models.Config, qm *queue.QueueManager) (*http.ServeMux, error) {
		return http.NewServeMux(), nil
	}
	started := make(chan struct{})
	stopped := make(chan struct{})
	httpServe = func(*http.Server) error {
		close(started)
		<-stopped
		return http.ErrServerClosed
	}
	shutdownErr := errors.New("shutdown failed")
	httpShutdown = func(*http.Server, context.Context) error {
		close(stopped)
		return shutdownErr
	}

	done := make(chan error, 1)
	go func() { done <- Run() }()
	<-started
	cancel()
	err := <-done
	if !errors.Is(err, shutdownErr) {
		t.Fatalf("Run() error = %v, want wrapped shutdown error", err)
	}
}

func TestMainFunc(t *testing.T) {
	oldExit := osExit
	t.Cleanup(func() { osExit = oldExit })
	stubRunContext(t)
	stubHTTPFunctions(t)

	// Test happy path
	httpServe = func(*http.Server) error { return nil }
	osExit = func(code int) { t.Errorf("osExit called with code %d", code) }
	main()

	// Test failure path
	httpServe = func(*http.Server) error { return fmt.Errorf("fail") }
	exitCalled := false
	osExit = func(code int) {
		exitCalled = true
		if code != 1 {
			t.Errorf("Expected code 1, got %d", code)
		}
	}
	main()
	if !exitCalled {
		t.Error("Expected osExit to be called")
	}
}
