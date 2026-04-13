package main

import (
	"fmt"
	"net/http"
	"os"
	"testing"

	"uplarr/internal/models"
	"uplarr/internal/queue"
)

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_VAR", "value")
	defer os.Unsetenv("TEST_VAR")
	if getEnv("TEST_VAR", "fallback") != "value" { t.Error("Expected value") }
	if getEnv("MISSING_VAR", "fallback") != "fallback" { t.Error("Expected fallback") }
}

func TestGetEnvInt(t *testing.T) {
	os.Setenv("TEST_INT", "123")
	defer os.Unsetenv("TEST_INT")
	if getEnvInt("TEST_INT", 0) != 123 { t.Error("Expected 123") }
	if getEnvInt("MISSING_INT", 456) != 456 { t.Error("Expected 456") }
	os.Setenv("INVALID_INT", "abc")
	if getEnvInt("INVALID_INT", 789) != 789 { t.Error("Expected fallback for invalid int") }
}

func TestRunSuccess(t *testing.T) {
	oldSetup := apiSetupApp
	oldListen := httpListenAndServe
	defer func() {
		apiSetupApp = oldSetup
		httpListenAndServe = oldListen
	}()

	apiSetupApp = func(config models.Config, qm *queue.QueueManager) (*http.ServeMux, error) {
		return http.NewServeMux(), nil
	}
	httpListenAndServe = func(addr string, handler http.Handler) error {
		return nil
	}

	if err := Run(); err != nil {
		t.Errorf("Expected success, got %v", err)
	}
}

func TestRunSetupFailure(t *testing.T) {
	oldSetup := apiSetupApp
	defer func() { apiSetupApp = oldSetup }()

	apiSetupApp = func(config models.Config, qm *queue.QueueManager) (*http.ServeMux, error) {
		return nil, fmt.Errorf("setup fail")
	}

	if err := Run(); err == nil {
		t.Error("Expected setup failure")
	}
}

func TestMainFunc(t *testing.T) {
	oldListen := httpListenAndServe
	oldExit := osExit
	defer func() {
		httpListenAndServe = oldListen
		osExit = oldExit
	}()

	// Test happy path
	httpListenAndServe = func(addr string, handler http.Handler) error { return nil }
	osExit = func(code int) { t.Errorf("osExit called with code %d", code) }
	main()

	// Test failure path
	httpListenAndServe = func(addr string, handler http.Handler) error { return fmt.Errorf("fail") }
	exitCalled := false
	osExit = func(code int) { 
		exitCalled = true 
		if code != 1 { t.Errorf("Expected code 1, got %d", code) }
	}
	main()
	if !exitCalled { t.Error("Expected osExit to be called") }
}
