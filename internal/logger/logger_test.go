package logger

import (
	"encoding/json"
	"testing"
	"time"
)

func TestLogger(t *testing.T) {
	// Setup test channel
	ch := make(chan string, 10)
	Mu.Lock()
	LogClients[ch] = true
	Mu.Unlock()

	defer func() {
		Mu.Lock()
		delete(LogClients, ch)
		Mu.Unlock()
		close(ch)
	}()

	// Test Info
	Info("test info message")
	select {
	case msg := <-ch:
		var parsed LogMessage
		if err := json.Unmarshal([]byte(msg), &parsed); err != nil {
			t.Fatalf("Failed to parse info log message: %v", err)
		}
		if parsed.Level != "info" {
			t.Errorf("Expected level 'info', got %q", parsed.Level)
		}
		if parsed.Msg != "test info message" {
			t.Errorf("Expected msg 'test info message', got %q", parsed.Msg)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for info log")
	}

	// Test Error
	Error("test error message")
	select {
	case msg := <-ch:
		var parsed LogMessage
		if err := json.Unmarshal([]byte(msg), &parsed); err != nil {
			t.Fatalf("Failed to parse error log message: %v", err)
		}
		if parsed.Level != "error" {
			t.Errorf("Expected level 'error', got %q", parsed.Level)
		}
		if parsed.Msg != "test error message" {
			t.Errorf("Expected msg 'test error message', got %q", parsed.Msg)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for error log")
	}

	// Test LogWithLevel directly
	LogWithLevel("custom", "custom message", map[string]string{"key": "value"})
	select {
	case msg := <-ch:
		var parsed LogMessage
		if err := json.Unmarshal([]byte(msg), &parsed); err != nil {
			t.Fatalf("Failed to parse custom log message: %v", err)
		}
		if parsed.Level != "custom" {
			t.Errorf("Expected level 'custom', got %q", parsed.Level)
		}
		if parsed.Msg != "custom message" {
			t.Errorf("Expected msg 'custom message', got %q", parsed.Msg)
		}
		// Assert metadata contains the "key":"value" entry
		if extra, ok := parsed.Extra.(map[string]interface{}); !ok {
			t.Errorf("Expected Extra to be a map, got %T", parsed.Extra)
		} else if extra["key"] != "value" {
			t.Errorf("Expected Extra[\"key\"] = \"value\", got %v", extra["key"])
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for custom log")
	}

	// Test BroadcastLog when client is blocked/full
	blockedCh := make(chan string, 1)
	Mu.Lock()
	LogClients[blockedCh] = true
	Mu.Unlock()

	blockedCh <- "fill"             // Fill the channel
	BroadcastLog("dropped message") // Should not block

	Mu.Lock()
	delete(LogClients, blockedCh)
	Mu.Unlock()
	close(blockedCh)
}
