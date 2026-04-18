package sftpclient

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"uplarr/internal/logger"
	"uplarr/internal/models"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/time/rate"
)

// DefaultMaxConcurrentRequestsPerFile controls the maximum number of concurrent
// outstanding requests per file during SFTP transfers. Raised from the library
// default of 64 to 128 for higher throughput on high-bandwidth links.
// Some SFTP servers with strict request/connection limits (e.g., certain
// ProFTPD mod_sftp or FileZilla Server configurations) may need a lower value.
// If you experience transfer failures or server disconnects, try reducing
// this to 64.
const DefaultMaxConcurrentRequestsPerFile = 128

type Limiter struct {
	rateLimiter    *rate.Limiter
	MaxLimit       rate.Limit
	MinLimit       rate.Limit
	MaxLatency     time.Duration
	lastLatency    time.Duration
	consecutiveLow int
	mu             sync.Mutex
}

func NewLimiter(limit, burst rate.Limit, minLimit rate.Limit, maxLatency time.Duration) *Limiter {
	return &Limiter{
		rateLimiter: rate.NewLimiter(limit, int(burst)),
		MaxLimit:    limit,
		MinLimit:    minLimit,
		MaxLatency:  maxLatency,
	}
}

func (l *Limiter) UpdateConfig(newLimit, newMinLimit rate.Limit, newMaxLatency time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// The UI uses 0 to mean "unlimited". Map that to rate.Inf so we don't
	// accidentally block all traffic (rate.Limit(0) = 0 events/sec).
	effective := newLimit
	if effective == 0 {
		effective = rate.Inf
	}

	l.MaxLimit = effective
	l.MinLimit = newMinLimit
	l.MaxLatency = newMaxLatency

	// When manually updating, we reset the current limit to the new MaxLimit.
	// This ensures that "Update Throttling" immediately takes effect even if previously throttled.
	l.rateLimiter.SetLimit(effective)
	l.consecutiveLow = 0
	if effective == rate.Inf {
		logger.Info(fmt.Sprintf("Throttling configuration updated: Speed unlimited, Min Speed %v KB/s, Max Latency %v", int(newMinLimit/1024), newMaxLatency))
	} else {
		logger.Info(fmt.Sprintf("Throttling configuration updated: Max Speed %v KB/s, Min Speed %v KB/s, Max Latency %v", int(effective/1024), int(newMinLimit/1024), newMaxLatency))
	}
}

func (l *Limiter) SetLimit(newLimit rate.Limit) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if newLimit > l.MaxLimit {
		newLimit = l.MaxLimit
	}
	l.rateLimiter.SetLimit(newLimit)
	l.consecutiveLow = 0
}

func (l *Limiter) Limit() rate.Limit {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.rateLimiter.Limit()
}

func (l *Limiter) RecordLatency(latency time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.lastLatency = latency
	currentLimit := l.rateLimiter.Limit()

	// Use our internal MaxLatency which is kept up-to-date by UpdateConfig
	maxLatency := l.MaxLatency
	if maxLatency <= 0 {
		return // Throttling disabled
	}

	if latency > maxLatency {
		newLimit := currentLimit * 0.8 // Less aggressive throttle down
		if newLimit < l.MinLimit {     // Use configurable floor
			newLimit = l.MinLimit
		}
		if newLimit < 1024 { // Absolute minimum floor (1 KB/s) to prevent stall
			newLimit = 1024
		}
		l.rateLimiter.SetLimit(newLimit)
		l.consecutiveLow = 0
		logger.Info(fmt.Sprintf("Latency high (%v > %v), throttling down to %v KB/s", latency, maxLatency, int(newLimit/1024)))
	} else if currentLimit < l.MaxLimit {
		l.consecutiveLow++
		if l.consecutiveLow >= 10 { // Faster recovery (10 instead of 20)
			newLimit := currentLimit * 1.2 // Faster pickup (20% instead of 10%)
			if newLimit > l.MaxLimit {
				newLimit = l.MaxLimit
			}
			l.rateLimiter.SetLimit(newLimit)
			l.consecutiveLow = 0
			logger.Info(fmt.Sprintf("Latency stable, increasing speed to %v KB/s", int(newLimit/1024)))
		}
	}
}

func (l *Limiter) WaitN(ctx context.Context, n int) error {
	return l.rateLimiter.WaitN(ctx, n)
}

func (l *Limiter) GetStats() (currentKB, maxKB int, lastLat time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return int(l.rateLimiter.Limit() / 1024), int(l.MaxLimit / 1024), l.lastLatency
}

func (l *Limiter) Burst() int {
	return l.rateLimiter.Burst()
}

type SFTPFile interface {
	io.ReadWriteCloser
	io.Seeker
	Stat() (os.FileInfo, error)
}

type SFTPClientInterface interface {
	Create(path string) (SFTPFile, error)
	OpenFile(path string, flags int) (SFTPFile, error)
	Stat(path string) (os.FileInfo, error)
	ReadDir(p string) ([]os.FileInfo, error)
	Remove(path string) error
	Rename(oldpath, newpath string) error
	Mkdir(path string) error
	Close() error
}

type realSFTPClient struct {
	*sftp.Client
}

func (c *realSFTPClient) Create(path string) (SFTPFile, error) {
	return c.Client.Create(path)
}

func (c *realSFTPClient) OpenFile(path string, flags int) (SFTPFile, error) {
	return c.Client.OpenFile(path, flags)
}

func (c *realSFTPClient) Stat(path string) (os.FileInfo, error) {
	return c.Client.Stat(path)
}

func (c *realSFTPClient) ReadDir(p string) ([]os.FileInfo, error) {
	return c.Client.ReadDir(p)
}

func (c *realSFTPClient) Remove(path string) error {
	return c.Client.Remove(path)
}

func (c *realSFTPClient) Rename(oldpath, newpath string) error {
	return c.Client.Rename(oldpath, newpath)
}

func (c *realSFTPClient) Mkdir(path string) error {
	return c.Client.Mkdir(path)
}

type SFTPClient struct {
	Host                    string
	Port                    string
	User                    string
	Password                string
	KeyPath                 string
	RemoteDir               string
	DeleteAfterVerify       bool
	Overwrite               bool
	KnownHostsPath          string
	SkipHostKeyVerification bool
	RateLimitKBps           int
	MaxLatencyMs            int
	MinLimitKBps            int
	ProgressCallback        func(bytesWritten int64)
	FileSizeCallback        func(totalBytes int64)
	Limiter                 *Limiter
	sshClient               *ssh.Client
	sftpClient              SFTPClientInterface
}

type progressWriter struct {
	w        io.Writer
	callback func(bytesWritten int64)
	total    int64
}

func (pw *progressWriter) Write(p []byte) (int, error) {
	n, err := pw.w.Write(p)
	if n > 0 {
		pw.total += int64(n)
		if pw.callback != nil {
			pw.callback(pw.total)
		}
	}
	return n, err
}

type throttledReader struct {
	ctx     context.Context
	r       io.Reader
	limiter *Limiter
}

func (tr *throttledReader) Read(p []byte) (n int, err error) {
	n, err = tr.r.Read(p)
	if n > 0 && tr.limiter != nil {
		// Apply byte-level rate limiting here.
		// Reading from local disk is extremely fast, so this WaitN
		// is decoupled from network RTT.
		burst := tr.limiter.Burst()
		remaining := n
		for remaining > 0 {
			waitN := remaining
			if waitN > burst {
				waitN = burst
			}
			if err := tr.limiter.WaitN(tr.ctx, waitN); err != nil {
				return n, err
			}
			remaining -= waitN
		}
	}
	return n, err
}

type throttledWriter struct {
	ctx        context.Context
	w          io.Writer
	limiter    *Limiter
	maxLatency time.Duration
}

func (tw *throttledWriter) Write(p []byte) (n int, err error) {
	// Check context first to satisfy cancellation tests
	select {
	case <-tw.ctx.Done():
		return 0, tw.ctx.Err()
	default:
	}

	// We no longer measure latency here because Write latency includes synchronous RTT
	// which causes false-positive throttling on high-latency links.
	// Latency is now measured in the background via TCP Ping.
	return tw.w.Write(p)
}

func (tw *throttledWriter) ReadFrom(r io.Reader) (int64, error) {
	if rf, ok := tw.w.(io.ReaderFrom); ok {
		return rf.ReadFrom(r)
	}
	return io.Copy(tw.w, r)
}

type contextWriter struct {
	ctx context.Context
	w   io.Writer
}

func (cw *contextWriter) Write(p []byte) (int, error) {
	select {
	case <-cw.ctx.Done():
		return 0, cw.ctx.Err()
	default:
		return cw.w.Write(p)
	}
}

var osReadFile = os.ReadFile
var sshParsePrivateKey = ssh.ParsePrivateKey

func (s *SFTPClient) SetLimiter(l *Limiter) {
	s.Limiter = l
}

func (s *SFTPClient) Connect() error {
	var authMethods []ssh.AuthMethod

	if s.KeyPath != "" {
		key, err := osReadFile(s.KeyPath)
		if err != nil {
			return fmt.Errorf("unable to read private key: %v", err)
		}
		signer, err := sshParsePrivateKey(key)
		if err != nil {
			return fmt.Errorf("unable to parse private key: %v", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	if s.Password != "" {
		authMethods = append(authMethods, ssh.Password(s.Password))
	}

	if len(authMethods) == 0 {
		return fmt.Errorf("no authentication methods available")
	}

	var hostKeyCallback ssh.HostKeyCallback
	if s.KnownHostsPath != "" {
		cb, err := knownhosts.New(s.KnownHostsPath)
		if err != nil {
			return fmt.Errorf("failed to load known hosts: %v", err)
		}
		hostKeyCallback = cb
	} else if s.SkipHostKeyVerification {
		hostKeyCallback = ssh.InsecureIgnoreHostKey() // #nosec G106
	} else {
		return fmt.Errorf("host key verification required (provide KnownHostsPath or SkipHostKeyVerification)")
	}

	config := &ssh.ClientConfig{
		User:            s.User,
		Auth:            authMethods,
		HostKeyCallback: hostKeyCallback,
		Timeout:         10 * time.Second,
	}

	addr := net.JoinHostPort(s.Host, s.Port)
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("failed to dial: %v", err)
	}
	s.sshClient = client

	sftpClient, err := sftp.NewClient(client,
		sftp.MaxConcurrentRequestsPerFile(DefaultMaxConcurrentRequestsPerFile),
		sftp.MaxPacket(32768),
	)
	if err != nil {
		_ = client.Close() // #nosec G104
		return fmt.Errorf("failed to create sftp client: %v", err)
	}
	s.sftpClient = &realSFTPClient{sftpClient}

	return nil
}

func (s *SFTPClient) Close() {
	if s.sftpClient != nil {
		_ = s.sftpClient.Close() // #nosec G104
	}
	if s.sshClient != nil {
		_ = s.sshClient.Close() // #nosec G104
	}
}

func (s *SFTPClient) GetRemoteDir() string {
	return s.RemoteDir
}

func (s *SFTPClient) UploadFileWithRetry(ctx context.Context, localPath string, maxRetries int) error {
	if maxRetries <= 0 {
		maxRetries = 1
	}
	var lastErr error
	const baseDelay = 100 * time.Millisecond
	const maxDelay = 5 * time.Second
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := s.UploadFile(ctx, localPath)
		if err == nil {
			return nil
		}
		lastErr = err
		logger.Error(fmt.Sprintf("Upload attempt %d failed for %s: %v", attempt, filepath.Base(localPath), err))
		if attempt < maxRetries {
			// Exponential backoff with jitter
			delay := baseDelay * time.Duration(1<<uint(attempt-1))
			if delay > maxDelay {
				delay = maxDelay
			}
			// Add jitter: ±25% of the delay
			jitter := time.Duration(rand.Int63n(int64(delay)/2)) - delay/4 // #nosec G404
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay + jitter):
			}
		}
	}
	return fmt.Errorf("upload failed after %d attempts: %w", maxRetries, lastErr)
}

func (s *SFTPClient) validateRemotePath(p string) (string, error) {
	if s.sftpClient == nil {
		return "", fmt.Errorf("SFTP client not connected")
	}
	// SFTP paths use forward slashes. Normalize and clean.
	p = path.Clean(filepath.ToSlash(p))

	// Determine the security base.
	// If the path is absolute, we allow navigation anywhere in the SFTP account (base = /).
	// If the path is relative, we jail it to the configured RemoteDir.
	base := "/"
	if !path.IsAbs(p) {
		base = path.Clean(filepath.ToSlash(s.RemoteDir))
		// Join joining the relative path with the base allows filepath.Rel
		// to correctly detect upward traversal on all platforms.
		p = path.Join(base, p)
	}

	rel, err := filepath.Rel(base, p)
	if err != nil {
		return "", fmt.Errorf("invalid remote path")
	}
	// Normalize to POSIX slashes since SFTP uses forward slashes
	rel = filepath.ToSlash(rel)

	// Precise escape check: rel == ".." or rel starts with "../"
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return "", fmt.Errorf("unauthorized remote path access: traversal detected")
	}

	return p, nil
}

func (s *SFTPClient) Remove(path string) error {
	p, err := s.validateRemotePath(path)
	if err != nil {
		return err
	}
	return s.sftpClient.Remove(p)
}

func (s *SFTPClient) Rename(oldpath, newpath string) error {
	op, err := s.validateRemotePath(oldpath)
	if err != nil {
		return err
	}
	np, err := s.validateRemotePath(newpath)
	if err != nil {
		return err
	}
	return s.sftpClient.Rename(op, np)
}

func (s *SFTPClient) Mkdir(path string) error {
	p, err := s.validateRemotePath(path)
	if err != nil {
		return err
	}
	return s.sftpClient.Mkdir(p)
}

func (s *SFTPClient) ReadRemoteDir(p string) ([]models.FileInfo, error) {
	cleanP, err := s.validateRemotePath(p)
	if err != nil {
		return nil, err
	}

	entries, err := s.sftpClient.ReadDir(cleanP)
	if err != nil {
		return nil, err
	}

	var fileInfos []models.FileInfo
	for _, entry := range entries {
		fileInfos = append(fileInfos, models.FileInfo{
			Name:  entry.Name(),
			Size:  entry.Size(),
			IsDir: entry.IsDir(),
		})
	}
	return fileInfos, nil
}

type File interface {
	io.ReadCloser
	io.Seeker
	Stat() (os.FileInfo, error)
}

var osOpen = func(name string) (File, error) {
	return os.Open(filepath.Clean(name)) // #nosec G304
}

var osRemove = os.Remove

func (s *SFTPClient) UploadFile(ctx context.Context, localPath string) (err error) {
	fileName := filepath.Base(localPath)
	// Ensure remote directory is normalized
	remoteDir := path.Clean(filepath.ToSlash(s.RemoteDir))
	remotePath := path.Join(remoteDir, fileName)

	// Remote security check
	if _, err := s.validateRemotePath(remotePath); err != nil {
		return err
	}

	tempRemotePath := remotePath + ".tmp"
	logger.Info(fmt.Sprintf("Copying file from %s -> %s", localPath, remotePath))

	// Check if file exists and handle overwrite
	if !s.Overwrite {
		_, err := s.sftpClient.Stat(remotePath)
		if err == nil {
			return fmt.Errorf("remote file %s already exists and overwrite is disabled", fileName)
		} else if !os.IsNotExist(err) && !strings.Contains(err.Error(), "file does not exist") {
			// If it's a real error (not just missing), return it
			return fmt.Errorf("failed to check remote file existence: %v", err)
		}
	}

	localFile, err := osOpen(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %v", err)
	}

	closed := false
	defer func() {
		if !closed {
			_ = localFile.Close()
		}
	}()

	localStat, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local file: %v", err)
	}

	var remoteFile SFTPFile
	var startOffset int64

	// Check for partial file to resume
	remoteStat, err := s.sftpClient.Stat(tempRemotePath)
	if err == nil {
		if remoteStat.Size() > 0 && remoteStat.Size() < localStat.Size() {
			logger.Info(fmt.Sprintf("Partial file found (%d/%d bytes). Attempting to resume...", remoteStat.Size(), localStat.Size()))
			rf, errOpen := s.sftpClient.OpenFile(tempRemotePath, os.O_RDWR)
			if errOpen == nil {
				// Verify prefix content matches to prevent corruption
				match := false
				remotePrefix := make([]byte, remoteStat.Size())
				_, errRead := io.ReadFull(rf, remotePrefix)
				if errRead == nil {
					localPrefix := make([]byte, remoteStat.Size())
					_, errLSeek := localFile.Seek(0, io.SeekStart)
					if errLSeek == nil {
						_, errLRead := io.ReadFull(localFile, localPrefix)
						if errLRead == nil && string(remotePrefix) == string(localPrefix) {
							match = true
						}
					}
				}

				if match {
					offset, errSeek := rf.Seek(0, io.SeekEnd)
					if errSeek == nil {
						_, errLSeek := localFile.Seek(offset, io.SeekStart)
						if errLSeek == nil {
							remoteFile = rf
							startOffset = offset
							logger.Info(fmt.Sprintf("Verfied prefix matches. Resuming from offset %d", startOffset))
						} else {
							logger.Error(fmt.Sprintf("Failed to seek local file: %v", errLSeek))
							_ = rf.Close()
						}
					} else {
						logger.Error(fmt.Sprintf("Failed to seek remote file: %v", errSeek))
						_ = rf.Close()
					}
				} else {
					logger.Warn(fmt.Sprintf("Partial file content mismatch for %s, restarting from zero.", tempRemotePath))
					_ = rf.Close()
					// We'll leave remoteFile nil so it gets Created (truncated) below
				}
			} else {
				logger.Error(fmt.Sprintf("Failed to open remote file for resume: %v", errOpen))
			}
		} else if remoteStat.Size() >= localStat.Size() {
			logger.Info(fmt.Sprintf("Existing partial file is larger than or equal to local file (%d >= %d), restarting.", remoteStat.Size(), localStat.Size()))
		}
	}

	if remoteFile == nil {
		remoteFile, err = s.sftpClient.Create(tempRemotePath)
		if err != nil {
			return fmt.Errorf("failed to create temp remote file: %v", err)
		}
	}

	cleanupRemote := true
	var verificationErr error
	defer func() {
		if remoteFile != nil {
			closeErr := remoteFile.Close()
			if err == nil && closeErr != nil {
				err = fmt.Errorf("failed to close remote file: %v", closeErr)
			}
		}
		if cleanupRemote {
			if verificationErr != nil {
				logger.Error(fmt.Sprintf("Verification failed for %s: %v. Partial file retained.", tempRemotePath, verificationErr))
			} else if ctx.Err() != nil {
				logger.Info(fmt.Sprintf("Transfer interrupted. Partial file retained at %s", tempRemotePath))
			} else if err != nil {
				logger.Error(fmt.Sprintf("Transfer failed for %s: %v. Partial file retained.", tempRemotePath, err))
			}
		}
	}()

	// Setup throttling and latency tracking
	if s.Limiter == nil && (s.RateLimitKBps > 0 || s.MaxLatencyMs > 0) {
		limit := rate.Limit(s.RateLimitKBps * 1024)
		if limit == 0 && s.MaxLatencyMs > 0 {
			limit = rate.Limit(100 * 1024 * 1024)
		}

		minLimit := rate.Limit(s.MinLimitKBps * 1024)
		if minLimit == 0 {
			minLimit = 10240 // Default 10 KB/s floor
		}

		burst := 16 * 1024
		if int(limit)/10 > burst {
			burst = int(limit) / 10
		}
		s.Limiter = NewLimiter(limit, rate.Limit(burst), minLimit, time.Duration(s.MaxLatencyMs)*time.Millisecond)
	}

	var targetWriter io.Writer = remoteFile
	var targetReader io.Reader = localFile

	// Byte-level throttling happens on the Reader side to avoid RTT interference
	if s.Limiter != nil {
		targetReader = &throttledReader{
			ctx:     ctx,
			r:       localFile,
			limiter: s.Limiter,
		}
	}

	// Latency measurement happens on the Writer side
	if s.Limiter != nil && s.MaxLatencyMs > 0 {
		targetWriter = &throttledWriter{
			ctx:        ctx,
			w:          remoteFile,
			limiter:    s.Limiter,
			maxLatency: time.Duration(s.MaxLatencyMs) * time.Millisecond,
		}
	}

	// Report total file size before upload begins
	if s.FileSizeCallback != nil {
		s.FileSizeCallback(localStat.Size())
	}

	// Wrap in progress tracker
	targetWriter = &progressWriter{w: targetWriter, callback: s.ProgressCallback, total: startOffset}
	if s.ProgressCallback != nil && startOffset > 0 {
		s.ProgressCallback(startOffset)
	}

	// Wrap in context checker
	targetWriter = &contextWriter{ctx: ctx, w: targetWriter}

	startTime := time.Now()

	// Start background latency sampler if dynamic throttling is enabled
	samplerCtx, samplerCancel := context.WithCancel(ctx)
	defer samplerCancel()
	if s.Limiter != nil && s.MaxLatencyMs > 0 {
		go s.startLatencySampler(samplerCtx)
	}

	// Use ReadFrom if available (provided by pkg/sftp.File for concurrent writes)
	if rf, ok := targetWriter.(io.ReaderFrom); ok {
		_, err = rf.ReadFrom(targetReader)
	} else {
		_, err = io.Copy(targetWriter, targetReader)
	}
	if err != nil {
		return fmt.Errorf("failed to copy file: %v", err)
	}

	duration := time.Since(startTime)
	logger.Info(fmt.Sprintf("Uploaded %s -> %s (%d bytes) in %s", localPath, remotePath, localStat.Size(), duration))

	// Verify the temp file
	remoteStat, err = s.sftpClient.Stat(tempRemotePath)
	if err != nil {
		return fmt.Errorf("failed to stat temp remote file for verification: %v", err)
	}

	if remoteStat.Size() != localStat.Size() {
		verificationErr = fmt.Errorf("size mismatch (local: %d, remote: %d)", localStat.Size(), remoteStat.Size())
		return fmt.Errorf("verification failed: %v", verificationErr)
	}

	// Success! Disable cleanup
	cleanupRemote = false
	logger.Info(fmt.Sprintf("Verification passed for %s", fileName+".tmp"))

	// Rename temp file to final file
	if s.Overwrite {
		_ = s.sftpClient.Remove(remotePath) // Best effort delete before rename to handle SFTP v3 rename restrictions
	}
	if err := s.sftpClient.Rename(tempRemotePath, remotePath); err != nil {
		return fmt.Errorf("failed to rename temp file to final destination: %v", err)
	}
	logger.Info(fmt.Sprintf("Successfully moved temp file to final destination: %s", fileName))

	// Windows requires file to be closed before deletion
	_ = localFile.Close()
	closed = true

	if s.DeleteAfterVerify {
		if err := osRemove(localPath); err != nil {
			logger.Error(fmt.Sprintf("Failed to delete local file %s: %v", localPath, err))
		} else {
			logger.Info(fmt.Sprintf("Deleted local file %s", fileName))
		}
	}

	return nil
}
func (s *SFTPClient) startLatencySampler(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	addr := net.JoinHostPort(s.Host, s.Port)
	const timeout = 2 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			start := time.Now()
			// TCP Ping: simply dial and close to measure RTT
			conn, err := net.DialTimeout("tcp", addr, timeout)
			latency := time.Since(start)
			if err == nil {
				_ = conn.Close()
				s.Limiter.RecordLatency(latency)
			} else {
				// If dial fails (e.g. timeout), record a latency that is guaranteed
				// to exceed the limiter's MaxLatency threshold so throttle-down
				// triggers regardless of the configured threshold.
				exceed := s.Limiter.MaxLatency + time.Millisecond
				s.Limiter.RecordLatency(exceed)
			}
		}
	}
}
