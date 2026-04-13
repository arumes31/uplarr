package logger

import (
	"encoding/json"
	"strings"
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
		if !strings.Contains(msg, "test info message") || !strings.Contains(msg, "info") {
			t.Errorf("Unexpected info log message: %s", msg)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for info log")
	}

	// Test Error
	Error("test error message")
	select {
	case msg := <-ch:
		if !strings.Contains(msg, "test error message") || !strings.Contains(msg, "error") {
			t.Errorf("Unexpected error log message: %s", msg)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for error log")
	}

	// Test LogWithLevel directly
	LogWithLevel("custom", "custom message", map[string]string{"key": "value"})
	select {
	case msg := <-ch:
		if !strings.Contains(msg, "custom message") || !strings.Contains(msg, "custom") || !strings.Contains(msg, "key") {
			t.Errorf("Unexpected custom log message: %s", msg)
		}
		var parsed LogMessage
		if err := json.Unmarshal([]byte(msg), &parsed); err != nil {
			t.Errorf("Failed to parse log message: %v", err)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for custom log")
	}

	// Test BroadcastLog when client is blocked/full
	blockedCh := make(chan string, 1)
	Mu.Lock()
	LogClients[blockedCh] = true
	Mu.Unlock()

	blockedCh <- "fill" // Fill the channel
	BroadcastLog("dropped message") // Should not block

	Mu.Lock()
	delete(LogClients, blockedCh)
	Mu.Unlock()
	close(blockedCh)
}
