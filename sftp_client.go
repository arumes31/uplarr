package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"golang.org/x/time/rate"
)

type SFTPFile interface {
	io.ReadWriteCloser
	Stat() (os.FileInfo, error)
}

type SFTPClientInterface interface {
	Create(path string) (SFTPFile, error)
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
	sshClient               *ssh.Client
	sftpClient              SFTPClientInterface
}

type throttledReader struct {
	ctx        context.Context
	r          io.Reader
	limiter    *rate.Limiter
	maxLatency time.Duration
}

func (tr *throttledReader) Read(p []byte) (n int, err error) {
	n, err = tr.r.Read(p)
	if n > 0 && tr.limiter != nil {
		start := time.Now()
		// If n exceeds burst, we must wait in chunks
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

		if tr.maxLatency > 0 {
			latency := time.Since(start)
			if latency > tr.maxLatency {
				// Throttle down: reduce limit by 10%
				currentLimit := tr.limiter.Limit()
				if currentLimit != rate.Inf {
					newLimit := currentLimit * 0.9
					if newLimit < 1024 { // Minimum 1KB/s
						newLimit = 1024
					}
					tr.limiter.SetLimit(newLimit)
					logInfo(fmt.Sprintf("Latency high (%v > %v), throttling down to %v KB/s", latency, tr.maxLatency, int(newLimit/1024)))
				}
			}
		}
	}
	return n, err
}

var osReadFile = os.ReadFile
var sshParsePrivateKey = ssh.ParsePrivateKey

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

	sftpClient, err := sftp.NewClient(client)
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

func (s *SFTPClient) UploadFileWithRetry(localPath string, maxRetries int) error {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := s.UploadFile(localPath)
		if err == nil {
			return nil
		}
		lastErr = err
		logError(fmt.Sprintf("Upload attempt %d failed for %s: %v", attempt, filepath.Base(localPath), err))
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("upload failed after %d attempts: %w", maxRetries, lastErr)
}

func (s *SFTPClient) ReadRemoteDir(p string) ([]FileInfo, error) {
	if s.sftpClient == nil {
		return nil, fmt.Errorf("SFTP client not connected")
	}
	
	entries, err := s.sftpClient.ReadDir(p)
	if err != nil {
		return nil, err
	}
	
	var fileInfos []FileInfo
	for _, entry := range entries {
		fileInfos = append(fileInfos, FileInfo{
			Name:  entry.Name(),
			Size:  entry.Size(),
			IsDir: entry.IsDir(),
		})
	}
	return fileInfos, nil
}

type File interface {
	io.ReadCloser
	Stat() (os.FileInfo, error)
}

var osOpen = func(name string) (File, error) {
	return os.Open(filepath.Clean(name)) // #nosec G304
}

var osRemove = os.Remove

func (s *SFTPClient) UploadFile(localPath string) error {
	fileName := filepath.Base(localPath)
	remotePath := path.Join(s.RemoteDir, fileName)

	// Check if file exists and handle overwrite
	if !s.Overwrite {
		_, err := s.sftpClient.Stat(remotePath)
		if err == nil {
			return fmt.Errorf("remote file %s already exists and overwrite is disabled", fileName)
		}
	}

	localFile, err := osOpen(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %v", err)
	}
	defer func() { _ = localFile.Close() }() // #nosec G104

	localStat, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local file: %v", err)
	}

	remoteFile, err := s.sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file: %v", err)
	}

	var reader io.Reader = localFile
	if s.RateLimitKBps > 0 || s.MaxLatencyMs > 0 {
		limit := rate.Limit(s.RateLimitKBps * 1024)
		if limit == 0 && s.MaxLatencyMs > 0 {
			// If only latency throttling is requested, start with a high limit
			limit = rate.Limit(100 * 1024 * 1024) // 100MB/s
		}
		
		if limit > 0 {
			// Burst is 16KB or at least 10% of the limit
			burst := 16 * 1024
			if int(limit)/10 > burst {
				burst = int(limit) / 10
			}
			limiter := rate.NewLimiter(limit, burst)
			reader = &throttledReader{
				ctx:        context.Background(),
				r:          localFile,
				limiter:    limiter,
				maxLatency: time.Duration(s.MaxLatencyMs) * time.Millisecond,
			}
		}
	}

	startTime := time.Now()
	_, err = io.Copy(remoteFile, reader)
	if err != nil {
		_ = remoteFile.Close() // #nosec G104
		return fmt.Errorf("failed to copy file: %v", err)
	}
	
	if err := remoteFile.Close(); err != nil {
		return fmt.Errorf("failed to close remote file: %v", err)
	}

	duration := time.Since(startTime)
	logInfo(fmt.Sprintf("Uploaded %s (%d bytes) in %s", fileName, localStat.Size(), duration))

	// Verify
	remoteStat, err := s.sftpClient.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("failed to stat remote file for verification: %v", err)
	}

	if remoteStat.Size() != localStat.Size() {
		return fmt.Errorf("verification failed: size mismatch (local: %d, remote: %d)", localStat.Size(), remoteStat.Size())
	}

	logInfo(fmt.Sprintf("Verification passed for %s", fileName))

	// Windows requires file to be closed before deletion
	_ = localFile.Close() // #nosec G104

	if s.DeleteAfterVerify {
		if err := osRemove(localPath); err != nil {
			logError(fmt.Sprintf("Failed to delete local file %s: %v", localPath, err))
		} else {
			logInfo(fmt.Sprintf("Deleted local file %s", fileName))
		}
	}

	return nil
}
