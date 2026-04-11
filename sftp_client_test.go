package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"github.com/pkg/sftp"
)

// --- Mocks ---

type mockSFTPFile struct {
	statSize int64
	statErr  error
	writeErr error
	closeErr error
	delay    time.Duration
	writeCnt int
	failAt   int // fail after this many writes
}

func (m *mockSFTPFile) Read(p []byte) (n int, err error)  { return 0, io.EOF }
func (m *mockSFTPFile) Write(p []byte) (n int, err error) { 
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	m.writeCnt++
	if m.failAt > 0 && m.writeCnt >= m.failAt {
		return 0, fmt.Errorf("interrupted transfer")
	}
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
	createFunc func(path string) (SFTPFile, error)
	statFunc   func(path string) (os.FileInfo, error)
	closeErr   error
}
func (m *mockSFTPClient) Create(path string) (SFTPFile, error) {
	if m.createFunc != nil { return m.createFunc(path) }
	return nil, fmt.Errorf("create not implemented")
}
func (m *mockSFTPClient) Stat(path string) (os.FileInfo, error) {
	if m.statFunc != nil { return m.statFunc(path) }
	return nil, fmt.Errorf("stat not implemented")
}
func (m *mockSFTPClient) Close() error { return m.closeErr }

type mockFileObj struct {
	*os.File
	statErr error
}
func (m *mockFileObj) Stat() (os.FileInfo, error) {
	if m.statErr != nil { return nil, m.statErr }
	return m.File.Stat()
}

// --- Helpers ---

func generateMockServerKey() ([]byte, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	privDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, err
	}
	privBlock := pem.Block{
		Type:    "PRIVATE KEY",
		Headers: nil,
		Bytes:   privDER,
	}
	return pem.EncodeToMemory(&privBlock), nil
}

func startMockSFTPServer(t *testing.T, user, password, uploadDir string) (string, func()) {
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) {
			if c.User() == user && string(p) == password {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected")
		},
		PublicKeyCallback: func(c ssh.ConnMetadata, pubKey ssh.PublicKey) (*ssh.Permissions, error) {
			if c.User() == user {
				return nil, nil
			}
			return nil, fmt.Errorf("key rejected")
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

// --- Tests ---

func TestSFTPClientConnect(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "sftp_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	port, cleanup := startMockSFTPServer(t, "user1", "pass1", tempDir)
	defer cleanup()

	// 1. Password Auth
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

	// 2. Invalid password
	clientInvalid := SFTPClient{
		Host:     "127.0.0.1",
		Port:     port,
		User:     "user1",
		Password: "wrong_password",
	}
	if err := clientInvalid.Connect(); err == nil {
		t.Fatal("Expected connect to fail with wrong password")
	}

	// 3. Public Key auth
	keyBytes, _ := generateMockServerKey()
	keyPath := filepath.Join(tempDir, "id_rsa")
	os.WriteFile(keyPath, keyBytes, 0600)

	clientKey := SFTPClient{
		Host:    "127.0.0.1",
		Port:    port,
		User:    "user1",
		KeyPath: keyPath,
	}
	if err := clientKey.Connect(); err != nil {
		t.Fatalf("Expected connect with key to succeed: %v", err)
	}
	clientKey.Close()
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

func TestSFTPClient_FullCoverage(t *testing.T) {
	mockFile := &mockSFTPFile{statSize: 10}
	mockC := &mockSFTPClient{
		createFunc: func(path string) (SFTPFile, error) { return mockFile, nil },
		statFunc:   func(path string) (os.FileInfo, error) { return &mockFileInfo{size: 10}, nil },
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
	oldCreate := mockC.createFunc
	mockC.createFunc = func(path string) (SFTPFile, error) { return nil, fmt.Errorf("create fail") }
	if err := client.UploadFile(testFile); err == nil || !strings.Contains(err.Error(), "create fail") {
		t.Errorf("Expected create fail, got %v", err)
	}
	mockC.createFunc = oldCreate

	// 4. io.Copy fail
	mockFile.writeErr = fmt.Errorf("write fail")
	if err := client.UploadFile(testFile); err == nil || !strings.Contains(err.Error(), "write fail") {
		t.Errorf("Expected write fail, got %v", err)
	}
	mockFile.writeErr = nil

	// 5. Remote Stat fail
	oldStat := mockC.statFunc
	mockC.statFunc = func(path string) (os.FileInfo, error) { return nil, fmt.Errorf("stat remote fail") }
	if err := client.UploadFile(testFile); err == nil || !strings.Contains(err.Error(), "stat remote fail") {
		t.Errorf("Expected stat remote fail, got %v", err)
	}
	mockC.statFunc = oldStat

	// 6. Size mismatch
	mockC.statFunc = func(path string) (os.FileInfo, error) { return &mockFileInfo{size: 5}, nil }
	if err := client.UploadFile(testFile); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Errorf("Expected size mismatch, got %v", err)
	}
	mockC.statFunc = oldStat

	// 7. Delete fail coverage
	client.DeleteAfterVerify = true
	oldRemove := osRemove
	osRemove = func(name string) error { return fmt.Errorf("remove error") }
	if err := client.UploadFile(testFile); err != nil {
		t.Errorf("Expected success even if remove fails, got %v", err)
	}
	
	// 8. Delete success coverage
	osRemove = func(name string) error { return nil }
	if err := client.UploadFile(testFile); err != nil {
		t.Errorf("Expected success on delete, got %v", err)
	}
	osRemove = oldRemove

	// 9. UploadFileWithRetry failure path
	osOpen = func(name string) (File, error) { return nil, fmt.Errorf("open fail") }
	if err := client.UploadFileWithRetry(testFile, 2); err == nil || !strings.Contains(err.Error(), "upload failed after 2 attempts") {
		t.Errorf("Expected retry fail, got %v", err)
	}
	osOpen = oldOpen
}

func TestSFTPClientUpload_AdvancedNetwork(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "adv_network")
	defer os.RemoveAll(tempDir)
	testFile := filepath.Join(tempDir, "test.txt")
	os.WriteFile(testFile, []byte("large data chunk for testing network issues"), 0644)

	mockC := &mockSFTPClient{}
	client := &SFTPClient{
		RemoteDir: ".",
		sftpClient: mockC,
	}

	// 1. Test Interrupted Transfer (Failure during Write)
	mockC.createFunc = func(path string) (SFTPFile, error) {
		return &mockSFTPFile{statSize: 43, failAt: 1}, nil
	}
	mockC.statFunc = func(path string) (os.FileInfo, error) {
		return &mockFileInfo{size: 43}, nil
	}

	err := client.UploadFile(testFile)
	if err == nil || !strings.Contains(err.Error(), "interrupted transfer") {
		t.Errorf("Expected interrupted transfer error, got %v", err)
	}

	// 2. Test Auto-Retry with eventual success
	attempt := 0
	mockC.createFunc = func(path string) (SFTPFile, error) {
		attempt++
		if attempt == 1 {
			return &mockSFTPFile{statSize: 43, failAt: 1}, nil
		}
		return &mockSFTPFile{statSize: 43}, nil
	}

	if err := client.UploadFileWithRetry(testFile, 2); err != nil {
		t.Errorf("Expected retry to succeed on second attempt, got %v", err)
	}

	// 3. Test High Latency
	mockC.createFunc = func(path string) (SFTPFile, error) {
		return &mockSFTPFile{statSize: 43, delay: 50 * time.Millisecond}, nil
	}
	
	start := time.Now()
	if err := client.UploadFile(testFile); err != nil {
		t.Errorf("Expected success with latency, got %v", err)
	}
	if time.Since(start) < 50*time.Millisecond {
		t.Errorf("Expected transfer to take at least 50ms, took %v", time.Since(start))
	}
}
