package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
	"github.com/pkg/sftp"
)

// generateMockServerKey generates a random RSA private key for the mock server.
func generateMockServerKey() ([]byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	privDER := x509.MarshalPKCS1PrivateKey(privateKey)
	privBlock := pem.Block{
		Type:    "RSA PRIVATE KEY",
		Headers: nil,
		Bytes:   privDER,
	}
	return pem.EncodeToMemory(&privBlock), nil
}

// startMockSFTPServer starts a local SFTP server and returns the port it's listening on and a cleanup func.
func startMockSFTPServer(t *testing.T, user, password, uploadDir string) (string, func()) {
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
			if c.User() == user && string(p) == password {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected")
		},
	}

	keyBytes, err := generateMockServerKey()
	if err != nil {
		t.Fatal(err)
	}

	private, err := ssh.ParsePrivateKey(keyBytes)
	if err != nil {
		t.Fatal(err)
	}
	config.AddHostKey(private)

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := strings.Split(listener.Addr().String(), ":")[1]

	go func() {
		for {
			nConn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(nConn net.Conn) {
				conn, chans, reqs, err := ssh.NewServerConn(nConn, config)
				if err != nil {
					return
				}
				go ssh.DiscardRequests(reqs)

				for newChannel := range chans {
					if newChannel.ChannelType() != "session" {
						newChannel.Reject(ssh.UnknownChannelType, "unknown channel type")
						continue
					}
					channel, requests, err := newChannel.Accept()
					if err != nil {
						continue
					}

					go func(in <-chan *ssh.Request) {
						for req := range in {
							ok := false
							if req.Type == "subsystem" && string(req.Payload[4:]) == "sftp" {
								ok = true
							}
							req.Reply(ok, nil)
						}
					}(requests)

					server, err := sftp.NewServer(channel, sftp.WithServerWorkingDirectory(uploadDir))
					if err == nil {
						server.Serve()
						server.Close()
					}
					conn.Close()
				}
			}(nConn)
		}
	}()

	return port, func() {
		listener.Close()
	}
}

func TestSFTPClientConnect(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sftp_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	port, cleanup := startMockSFTPServer(t, "user1", "pass1", tempDir)
	defer cleanup()

	// Test Password Auth
	client := SFTPClient{
		Host:     "127.0.0.1",
		Port:     port,
		User:     "user1",
		Password: "pass1",
	}
	if err := client.Connect(); err != nil {
		t.Fatalf("Expected connect to succeed: %v", err)
	}
	client.Close()

	// Test invalid password
	clientInvalid := SFTPClient{
		Host:     "127.0.0.1",
		Port:     port,
		User:     "user1",
		Password: "wrong_password",
	}
	if err := clientInvalid.Connect(); err == nil {
		t.Fatal("Expected connect to fail with wrong password")
	}
}

func TestSFTPClientUpload(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "sftp_upload_test")
	defer os.RemoveAll(tempDir)
	remoteDir := filepath.Join(tempDir, "remote")
	os.MkdirAll(remoteDir, 0755)
	localDir := filepath.Join(tempDir, "local")
	os.MkdirAll(localDir, 0755)

	port, cleanup := startMockSFTPServer(t, "user1", "pass1", remoteDir)
	defer cleanup()

	client := SFTPClient{
		Host:      "127.0.0.1",
		Port:      port,
		User:      "user1",
		Password:  "pass1",
		RemoteDir: ".",
	}
	client.Connect()
	defer client.Close()

	testFile := filepath.Join(localDir, "test.txt")
	os.WriteFile(testFile, []byte("hello world"), 0644)

	if err := client.UploadFile(testFile); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	if err := client.UploadFileWithRetry(testFile, 1); err != nil {
		t.Fatalf("UploadFileWithRetry failed: %v", err)
	}
}

// Mocking for 100% coverage
type mockSFTPFile struct {
	statSize int64
	statErr  error
	writeErr error
	closeErr error
}
func (m *mockSFTPFile) Read(p []byte) (n int, err error)  { return 0, io.EOF }
func (m *mockSFTPFile) Write(p []byte) (n int, err error) { 
	if m.writeErr != nil { return 0, m.writeErr }
	return len(p), nil 
}
func (m *mockSFTPFile) Close() error { return m.closeErr }
func (m *mockSFTPFile) Stat() (os.FileInfo, error) {
	if m.statErr != nil { return nil, m.statErr }
	return &mockFileInfo{size: m.statSize}, nil
}

type mockFileInfo struct {
	os.FileInfo
	size int64
}
func (m *mockFileInfo) Size() int64 { return m.size }

type mockSFTPClient struct {
	createFile *mockSFTPFile
	createErr  error
	statFile   *mockFileInfo
	statErr    error
	closeErr   error
}
func (m *mockSFTPClient) Create(path string) (SFTPFile, error) {
	if m.createErr != nil { return nil, m.createErr }
	return m.createFile, nil
}
func (m *mockSFTPClient) Stat(path string) (os.FileInfo, error) {
	if m.statErr != nil { return nil, m.statErr }
	return m.statFile, nil
}
func (m *mockSFTPClient) Close() error { return m.closeErr }

func TestSFTPClient_FullCoverage(t *testing.T) {
	mockC := &mockSFTPClient{
		createFile: &mockSFTPFile{statSize: 10},
		statFile:   &mockFileInfo{size: 10},
	}
	client := &SFTPClient{
		RemoteDir: ".",
		sftpClient: mockC,
	}

	tempDir, _ := os.MkdirTemp("", "full_cov")
	defer os.RemoveAll(tempDir)
	testFile := filepath.Join(tempDir, "test.txt")
	os.WriteFile(testFile, []byte("1234567890"), 0644)

	// 1. Success
	if err := client.UploadFile(testFile); err != nil {
		t.Errorf("Expected success, got %v", err)
	}

	// 2. Stat local fail
	oldOpen := osOpen
	osOpen = func(name string) (File, error) {
		f, _ := os.Open(name)
		return &mockFileObj{File: f, statErr: fmt.Errorf("stat fail")}, nil
	}
	if err := client.UploadFile(testFile); err == nil || !strings.Contains(err.Error(), "stat fail") {
		t.Errorf("Expected stat fail, got %v", err)
	}
	osOpen = oldOpen

	// 3. Create remote fail
	mockC.createErr = fmt.Errorf("create fail")
	if err := client.UploadFile(testFile); err == nil || !strings.Contains(err.Error(), "create fail") {
		t.Errorf("Expected create fail, got %v", err)
	}
	mockC.createErr = nil

	// 4. io.Copy fail
	mockC.createFile.writeErr = fmt.Errorf("write fail")
	if err := client.UploadFile(testFile); err == nil || !strings.Contains(err.Error(), "write fail") {
		t.Errorf("Expected write fail, got %v", err)
	}
	mockC.createFile.writeErr = nil

	// 5. Remote Stat fail
	mockC.statErr = fmt.Errorf("stat remote fail")
	if err := client.UploadFile(testFile); err == nil || !strings.Contains(err.Error(), "stat remote fail") {
		t.Errorf("Expected stat remote fail, got %v", err)
	}
	mockC.statErr = nil

	// 6. Size mismatch
	mockC.statFile.size = 5
	if err := client.UploadFile(testFile); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Errorf("Expected size mismatch, got %v", err)
	}
	mockC.statFile.size = 10
}

func TestSFTPClientConnect_MockErrors(t *testing.T) {
	client := &SFTPClient{KeyPath: "somepath", User: "u", Host: "h", Port: "p"}

	// 1. ReadFile fail
	oldRead := osReadFile
	osReadFile = func(name string) ([]byte, error) { return nil, fmt.Errorf("read fail") }
	if err := client.Connect(); err == nil || !strings.Contains(err.Error(), "read fail") {
		t.Errorf("Expected read fail, got %v", err)
	}
	osReadFile = oldRead

	// 2. ParsePrivateKey fail
	oldParse := sshParsePrivateKey
	sshParsePrivateKey = func(b []byte) (ssh.Signer, error) { return nil, fmt.Errorf("parse fail") }
	osReadFile = func(name string) ([]byte, error) { return []byte("key"), nil }
	if err := client.Connect(); err == nil || !strings.Contains(err.Error(), "parse fail") {
		t.Errorf("Expected parse fail, got %v", err)
	}
	sshParsePrivateKey = oldParse
	osReadFile = oldRead
	
	// 3. No auth methods
	clientNoAuth := &SFTPClient{User: "u", Host: "h", Port: "p"}
	if err := clientNoAuth.Connect(); err == nil || !strings.Contains(err.Error(), "no authentication methods") {
		t.Errorf("Expected no auth methods error, got %v", err)
	}
}

type mockFileObj struct {
	*os.File
	statErr error
}
func (m *mockFileObj) Stat() (os.FileInfo, error) {
	if m.statErr != nil { return nil, m.statErr }
	return m.File.Stat()
}
