package main

import (
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
)

type SFTPFile interface {
	io.ReadWriteCloser
	Stat() (os.FileInfo, error)
}

type SFTPClientInterface interface {
	Create(path string) (SFTPFile, error)
	Stat(path string) (os.FileInfo, error)
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

type SFTPClient struct {
	Host                    string
	Port                    string
	User                    string
	Password                string
	KeyPath                 string
	RemoteDir               string
	DeleteAfterVerify       bool
	KnownHostsPath          string
	SkipHostKeyVerification bool
	sshClient               *ssh.Client
	sftpClient              SFTPClientInterface
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

	startTime := time.Now()
	_, err = io.Copy(remoteFile, localFile)
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
