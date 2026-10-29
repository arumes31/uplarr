package sftpclient

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	"uplarr/internal/logger"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Sentinel errors returned by Connect. Callers that answer HTTP requests use
// them to report a useful but generic reason without echoing dial or file
// system details back to the client.
var (
	ErrHostKeyRequired  = errors.New("host key verification required")
	ErrHostKeyUnknown   = errors.New("host key is not trusted")
	ErrHostKeyMismatch  = errors.New("host key does not match the pinned key")
	ErrPrivateKey       = errors.New("private key could not be loaded")
	ErrNoAuthMethods    = errors.New("no authentication methods available")
	ErrDialTargetDenied = errors.New("connection target is not allowed")
)

// hostSecurity holds process-wide settings configured once at startup.
var (
	hostSecurityMu sync.RWMutex
	knownHostsFile string
	trustOnFirst   bool
	keysDir        string

	// knownHostsWriteMu serialises trust-on-first-use writes so concurrent
	// uploads to a new host cannot append duplicate or interleaved lines.
	knownHostsWriteMu sync.Mutex
)

// SetKnownHostsFile configures the known_hosts file used when a client has no
// explicit KnownHostsPath. With tofu enabled, keys of hosts that are not yet
// listed are pinned on first connect; a changed key always fails.
func SetKnownHostsFile(path string, tofu bool) {
	hostSecurityMu.Lock()
	defer hostSecurityMu.Unlock()
	knownHostsFile = path
	trustOnFirst = tofu
}

// SetKeysDir configures the only directory private keys may be read from.
func SetKeysDir(dir string) {
	hostSecurityMu.Lock()
	defer hostSecurityMu.Unlock()
	keysDir = dir
}

func hostSecuritySettings() (string, bool, string) {
	hostSecurityMu.RLock()
	defer hostSecurityMu.RUnlock()
	return knownHostsFile, trustOnFirst, keysDir
}

// readKeyFile reads name from inside dir through os.Root so that symlinks or
// ".." cannot escape the keys directory.
var readKeyFile = func(dir, name string) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer func() { _ = root.Close() }()
	return root.ReadFile(name)
}

// loadPrivateKey resolves keyPath inside the configured keys directory. A bare
// file name or a path that already points inside the directory is accepted;
// anything else is rejected without revealing whether the file exists.
func loadPrivateKey(keyPath string) ([]byte, error) {
	_, _, dir := hostSecuritySettings()
	if dir == "" {
		return nil, fmt.Errorf("%w: no keys directory configured", ErrPrivateKey)
	}

	name := keyPath
	if filepath.IsAbs(keyPath) {
		absDir, err := filepath.Abs(dir)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrPrivateKey, err)
		}
		rel, err := filepath.Rel(absDir, keyPath)
		if err != nil {
			return nil, fmt.Errorf("%w: path is outside the keys directory", ErrPrivateKey)
		}
		name = rel
	}
	if !filepath.IsLocal(name) {
		return nil, fmt.Errorf("%w: path is outside the keys directory", ErrPrivateKey)
	}

	data, err := readKeyFile(dir, name)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPrivateKey, err)
	}
	return data, nil
}

// tofuHostKeyCallback verifies host keys against path, pinning unknown hosts
// when tofu is enabled. A key that differs from a pinned one is always fatal.
func tofuHostKeyCallback(path string, tofu bool) ssh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key ssh.PublicKey) error {
		knownHostsWriteMu.Lock()
		defer knownHostsWriteMu.Unlock()

		f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0600) // #nosec G304 -- path comes from server configuration
		if err != nil {
			return fmt.Errorf("failed to open known hosts: %w", err)
		}
		_ = f.Close()

		cb, err := knownhosts.New(path)
		if err != nil {
			return fmt.Errorf("failed to load known hosts: %w", err)
		}

		err = cb(hostname, remote, key)
		var keyErr *knownhosts.KeyError
		switch {
		case err == nil:
			return nil
		case errors.As(err, &keyErr) && len(keyErr.Want) > 0:
			logger.Error(fmt.Sprintf("Host key for %s changed (got %s); refusing to connect", hostname, ssh.FingerprintSHA256(key)))
			return fmt.Errorf("%w: %s presented %s", ErrHostKeyMismatch, hostname, ssh.FingerprintSHA256(key))
		case errors.As(err, &keyErr):
			if !tofu {
				return fmt.Errorf("%w: %s presented %s", ErrHostKeyUnknown, hostname, ssh.FingerprintSHA256(key))
			}
		default:
			return err
		}

		line := knownhosts.Line([]string{knownhosts.Normalize(hostname)}, key)
		af, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600) // #nosec G304 -- path comes from server configuration
		if err != nil {
			return fmt.Errorf("failed to pin host key: %w", err)
		}
		if _, err := af.WriteString(line + "\n"); err != nil {
			_ = af.Close()
			return fmt.Errorf("failed to pin host key: %w", err)
		}
		if err := af.Close(); err != nil {
			return fmt.Errorf("failed to pin host key: %w", err)
		}
		logger.Info(fmt.Sprintf("Trusted new host key for %s on first use: %s %s", hostname, key.Type(), ssh.FingerprintSHA256(key)))
		return nil
	}
}

// allowLinkLocal reports whether dialing link-local addresses (which include
// cloud metadata endpoints such as 169.254.169.254) has been enabled.
func allowLinkLocal() bool {
	return strings.EqualFold(os.Getenv("UPLARR_ALLOW_LINK_LOCAL"), "true")
}

// checkDialTarget runs after name resolution, on the address actually being
// dialed, so DNS tricks cannot bypass it.
func checkDialTarget(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return ErrDialTargetDenied
	}
	if ip.IsUnspecified() || ip.IsMulticast() {
		return ErrDialTargetDenied
	}
	if (ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()) && !allowLinkLocal() {
		return ErrDialTargetDenied
	}
	return nil
}

// dialSSH is ssh.Dial with a guarded dialer.
func dialSSH(addr string, config *ssh.ClientConfig) (*ssh.Client, error) {
	d := net.Dialer{Timeout: config.Timeout, Control: checkDialTarget}
	conn, err := d.DialContext(context.Background(), "tcp", addr)
	if err != nil {
		return nil, err
	}
	c, chans, reqs, err := ssh.NewClientConn(conn, addr, config)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return ssh.NewClient(c, chans, reqs), nil
}

// PublicConnectError maps an SFTP connection error to a message that is safe
// to return to the client. Raw dial errors are only logged server side so the
// endpoints cannot be used to fingerprint internal hosts and ports.
func PublicConnectError(err error) string {
	// %q escapes newlines from resolver errors that echo the requested host.
	log.Printf("SFTP connection failed: %q", err.Error())
	switch {
	case errors.Is(err, ErrHostKeyMismatch):
		return "Host key verification failed: the server key does not match the pinned key"
	case errors.Is(err, ErrHostKeyUnknown):
		return "Host key verification failed: the server key is not trusted"
	case errors.Is(err, ErrHostKeyRequired):
		return "Host key verification required"
	case errors.Is(err, ErrPrivateKey):
		return "Private key could not be loaded from the keys directory"
	case errors.Is(err, ErrNoAuthMethods):
		return "No authentication methods available"
	case errors.Is(err, ErrDialTargetDenied):
		return "Connection target is not allowed"
	case strings.Contains(err.Error(), "unable to authenticate"):
		return "Authentication failed"
	default:
		return "Connection failed"
	}
}
