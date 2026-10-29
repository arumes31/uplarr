package sftpclient

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func testPublicKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pemBytes, err := generateMockServerKey()
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.ParsePrivateKey(pemBytes)
	if err != nil {
		t.Fatal(err)
	}
	return signer.PublicKey()
}

func TestTOFUHostKeyCallback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	addr := &net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 22}
	first := testPublicKey(t)
	second := testPublicKey(t)

	strict := tofuHostKeyCallback(path, false)
	if err := strict("sftp.example:22", addr, first); !errors.Is(err, ErrHostKeyUnknown) {
		t.Fatalf("expected unknown host error without TOFU, got %v", err)
	}

	cb := tofuHostKeyCallback(path, true)
	if err := cb("sftp.example:22", addr, first); err != nil {
		t.Fatalf("expected first key to be pinned, got %v", err)
	}
	if err := cb("sftp.example:22", addr, first); err != nil {
		t.Fatalf("expected pinned key to verify, got %v", err)
	}
	if err := strict("sftp.example:22", addr, first); err != nil {
		t.Fatalf("expected pinned key to verify in strict mode, got %v", err)
	}
	if err := cb("sftp.example:22", addr, second); !errors.Is(err, ErrHostKeyMismatch) {
		t.Fatalf("expected mismatch for changed key, got %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(strings.Split(strings.TrimSpace(string(data)), "\n")); got != 1 {
		t.Fatalf("expected exactly one pinned line, got %d", got)
	}
}

func TestLoadPrivateKeyConfined(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "id_ed25519"), []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}

	SetKeysDir("")
	if _, err := loadPrivateKey("id_ed25519"); !errors.Is(err, ErrPrivateKey) {
		t.Fatalf("expected error without keys dir, got %v", err)
	}

	SetKeysDir(dir)
	t.Cleanup(func() { SetKeysDir("") })

	for _, ok := range []string{"id_ed25519", filepath.Join(dir, "id_ed25519")} {
		if data, err := loadPrivateKey(ok); err != nil || string(data) != "key" {
			t.Errorf("loadPrivateKey(%q) = %q, %v; want key", ok, data, err)
		}
	}
	for _, bad := range []string{outside, "../secret", filepath.Join("..", filepath.Base(filepath.Dir(outside)), "secret"), "missing"} {
		if _, err := loadPrivateKey(bad); !errors.Is(err, ErrPrivateKey) {
			t.Errorf("loadPrivateKey(%q) error = %v; want ErrPrivateKey", bad, err)
		}
	}
}

func TestCheckDialTarget(t *testing.T) {
	tests := []struct {
		addr    string
		allowed bool
	}{
		{"192.0.2.1:22", true},
		{"127.0.0.1:22", true},
		{"10.0.0.5:22", true},
		{"[2001:db8::1]:22", true},
		{"169.254.169.254:80", false},
		{"[fe80::1]:22", false},
		{"0.0.0.0:22", false},
		{"224.0.0.1:22", false},
		{"not-an-ip:22", false},
		{"missing-port", false},
	}
	for _, tc := range tests {
		err := checkDialTarget("tcp", tc.addr, nil)
		if (err == nil) != tc.allowed {
			t.Errorf("checkDialTarget(%q) = %v; allowed=%v", tc.addr, err, tc.allowed)
		}
	}

	t.Setenv("UPLARR_ALLOW_LINK_LOCAL", "true")
	if err := checkDialTarget("tcp", "169.254.169.254:22", nil); err != nil {
		t.Errorf("expected link-local to be allowed when opted in, got %v", err)
	}
}

func TestConnectUsesConfiguredKnownHosts(t *testing.T) {
	SetKnownHostsFile(filepath.Join(t.TempDir(), "known_hosts"), true)
	t.Cleanup(func() { SetKnownHostsFile("", false) })

	// Dialing a link-local address fails before any network traffic, which
	// proves the guarded dialer is in use once host key setup has passed.
	c := &SFTPClient{Host: "169.254.169.254", Port: "22", User: "u", Password: "p"}
	if err := c.Connect(); !errors.Is(err, ErrDialTargetDenied) {
		t.Fatalf("expected dial target denial, got %v", err)
	}
}

func TestRetryDelayStaysBounded(t *testing.T) {
	for _, attempt := range []int{1, 2, 10, 63, 64, 65, 1000, 1 << 30} {
		d := retryDelay(attempt)
		if d <= 0 || d > 5*time.Second*5/4 {
			t.Fatalf("attempt %d: delay %v out of range", attempt, d)
		}
	}
}
