package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

type SFTPClient struct {
	Host              string
	Port              string
	User              string
	Password          string
	KeyPath           string
	RemoteDir         string
	DeleteAfterVerify bool
	sshClient         *ssh.Client
	sftpClient        *sftp.Client
}

func (s *SFTPClient) Connect() error {
	var authMethods []ssh.AuthMethod

	if s.KeyPath != "" {
		key, err := os.ReadFile(s.KeyPath)
		if err != nil {
			return fmt.Errorf("unable to read private key: %v", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
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

	config := &ssh.ClientConfig{
		User:            s.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
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
		client.Close()
		return fmt.Errorf("failed to create sftp client: %v", err)
	}
	s.sftpClient = sftpClient

	return nil
}

func (s *SFTPClient) Close() {
	if s.sftpClient != nil {
		s.sftpClient.Close()
	}
	if s.sshClient != nil {
		s.sshClient.Close()
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
		log.Printf(`{"level":"warn", "msg":"Upload attempt failed", "attempt":%d, "file":"%s", "error":"%v"}`, attempt, filepath.Base(localPath), err)
		time.Sleep(100 * time.Millisecond) // short backoff for testing and fast retries
	}
	return fmt.Errorf("upload failed after %d attempts: %w", maxRetries, lastErr)
}

func (s *SFTPClient) UploadFile(localPath string) error {
	fileName := filepath.Base(localPath)
	remotePath := filepath.Join(s.RemoteDir, fileName)

	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("failed to open local file: %v", err)
	}
	defer localFile.Close()

	localStat, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat local file: %v", err)
	}

	remoteFile, err := s.sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("failed to create remote file: %v", err)
	}
	defer remoteFile.Close()

	startTime := time.Now()
	_, err = io.Copy(remoteFile, localFile)
	if err != nil {
		return fmt.Errorf("failed to copy file: %v", err)
	}
	duration := time.Since(startTime)

	log.Printf(`{"level":"info", "msg":"Uploaded file", "file":"%s", "size":%d, "duration":"%s"}`, fileName, localStat.Size(), duration)

	// Verify
	remoteStat, err := s.sftpClient.Stat(remotePath)
	if err != nil {
		return fmt.Errorf("failed to stat remote file for verification: %v", err)
	}

	if remoteStat.Size() != localStat.Size() {
		return fmt.Errorf("verification failed: size mismatch (local: %d, remote: %d)", localStat.Size(), remoteStat.Size())
	}

	log.Printf(`{"level":"info", "msg":"Verification passed", "file":"%s"}`, fileName)

	// Windows requires file to be closed before deletion
	localFile.Close()

	if s.DeleteAfterVerify {
		if err := os.Remove(localPath); err != nil {
			log.Printf(`{"level":"error", "msg":"Failed to delete local file", "file":"%s", "error":"%v"}`, localPath, err)
		} else {
			log.Printf(`{"level":"info", "msg":"Deleted local file", "file":"%s"}`, fileName)
		}
	}

	return nil
}
