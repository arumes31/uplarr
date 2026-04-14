package sftpclient

import (
	"context"
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

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/time/rate"
)

// --- Mocks ---

type mockSFTPFile struct {
	statSize int64
	statErr  error
	writeErr error
	closeErr error
	delay    time.Duration
	writeCnt int
	failAt   int
	pos      int64
	content  []byte
}

func (m *mockSFTPFile) Read(p []byte) (n int, err error) {
	if m.pos >= int64(len(m.content)) {
		return 0, io.EOF
	}
	n = copy(p, m.content[m.pos:])
	m.pos += int64(n)
	return n, nil
}
func (m *mockSFTPFile) Write(p []byte) (n int, err error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	m.writeCnt++
	if m.failAt > 0 && m.writeCnt >= m.failAt {
		return 0, fmt.Errorf("interrupted transfer")
	}
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	return len(p), nil
}
func (m *mockSFTPFile) Close() error { return m.closeErr }
func (m *mockSFTPFile) Seek(offset int64, whence int) (int64, error) {
	var newPos int64
	switch whence {
	case io.SeekStart:
		newPos = offset
	case io.SeekCurrent:
		newPos = m.pos + offset
	case io.SeekEnd:
		newPos = int64(len(m.content)) + offset
	default:
		return 0, fmt.Errorf("invalid whence")
	}
	if newPos < 0 {
		return 0, fmt.Errorf("negative position")
	}
	m.pos = newPos
	return m.pos, nil
}
func (m *mockSFTPFile) Stat() (os.FileInfo, error) {
	if m.statErr != nil {
		return nil, m.statErr
	}
	return &mockFileInfo{size: m.statSize}, nil
}

type mockFileInfo struct {
	size    int64
	name    string
	isDir   bool
	mode    os.FileMode
	modTime time.Time
}

func (m *mockFileInfo) Size() int64        { return m.size }
func (m *mockFileInfo) Name() string       { return m.name }
func (m *mockFileInfo) IsDir() bool        { return m.isDir }
func (m *mockFileInfo) Mode() os.FileMode  { return m.mode }
func (m *mockFileInfo) ModTime() time.Time { return m.modTime }
func (m *mockFileInfo) Sys() interface{}   { return nil }

type mockSFTPClient struct {
	createFunc  func(path string) (SFTPFile, error)
	openFileFunc func(path string, flags int) (SFTPFile, error)
	statFunc     func(path string) (os.FileInfo, error)
	readDirFunc  func(p string) ([]os.FileInfo, error)
	mkdirErr     error
	removeErr    error
	renameErr    error
	closeErr     error
	files        map[string]*mockSFTPFile
}

func (m *mockSFTPClient) Create(path string) (SFTPFile, error) {
	if m.createFunc != nil {
		return m.createFunc(path)
	}
	return nil, fmt.Errorf("create not implemented")
}
func (m *mockSFTPClient) OpenFile(path string, flags int) (SFTPFile, error) {
	if m.openFileFunc != nil {
		return m.openFileFunc(path, flags)
	}
	if f, ok := m.files[path]; ok {
		return f, nil
	}
	return nil, fmt.Errorf("openfile: file not found in mock: %s", path)
}
func (m *mockSFTPClient) Stat(path string) (os.FileInfo, error) {
	if m.statFunc != nil {
		return m.statFunc(path)
	}
	if f, ok := m.files[path]; ok {
		return &mockFileInfo{size: f.statSize, name: filepath.Base(path)}, nil
	}
	return nil, os.ErrNotExist
}
func (m *mockSFTPClient) ReadDir(p string) ([]os.FileInfo, error) {
	if m.readDirFunc != nil {
		return m.readDirFunc(p)
	}
	return []os.FileInfo{}, nil
}
func (m *mockSFTPClient) Mkdir(path string) error { return m.mkdirErr }
func (m *mockSFTPClient) Remove(path string) error { return m.removeErr }
func (m *mockSFTPClient) Rename(oldpath, newpath string) error { return m.renameErr }
func (m *mockSFTPClient) Close() error { return m.closeErr }

type mockFileObj struct {
	io.ReadCloser
	osFile  *os.File
	statErr error
}

func (m *mockFileObj) Stat() (os.FileInfo, error) {
	if m.statErr != nil {
		return nil, m.statErr
	}
	return m.osFile.Stat()
}
func (m *mockFileObj) Close() error { return m.osFile.Close() }
func (m *mockFileObj) Read(p []byte) (n int, err error) { return m.osFile.Read(p) }
func (m *mockFileObj) Seek(offset int64, whence int) (int64, error) { return m.osFile.Seek(offset, whence) }

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
		PublicKeyCallback: func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			return nil, nil
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

	stop := make(chan struct{})
	go func() {
		for {
			select {
			case <-stop:
				return
			default:
				nConn, err := listener.Accept()
				if err != nil {
					return
				}
				go func(nConn net.Conn) {
					defer nConn.Close()
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
		}
	}()

	return port, func() {
		close(stop)
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

	// 1. Password Auth + Skip Verification
	client := SFTPClient{
		Host:                    "127.0.0.1",
		Port:                    port,
		User:                    "user1",
		Password:                "pass1",
		SkipHostKeyVerification: true,
	}
	if err := client.Connect(); err != nil {
		t.Fatalf("Expected connect to succeed: %v", err)
	}
	
	if client.sftpClient != nil {
		_ = client.sftpClient.Mkdir("testdir")
		_, _ = client.sftpClient.Stat("testdir")
		_, _ = client.sftpClient.ReadDir(".")
		f, _ := client.sftpClient.Create("test.txt")
		if f != nil {
			_, _ = f.Stat()
			_ = f.Close()
		}
		_ = client.sftpClient.Rename("test.txt", "test2.txt")
		_ = client.sftpClient.Remove("test2.txt")
	}
	
	client.Close()

	// 2. Missing Host Key Verification
	clientNoVerify := SFTPClient{
		Host:     "127.0.0.1",
		Port:     port,
		User:     "user1",
		Password: "pass1",
	}
	if err := clientNoVerify.Connect(); err == nil || !strings.Contains(err.Error(), "host key verification required") {
		t.Errorf("Expected verification error, got %v", err)
	}

	// 3. Public Key auth
	keyBytes, _ := generateMockServerKey()
	keyPath := filepath.Join(tempDir, "id_rsa")
	os.WriteFile(keyPath, keyBytes, 0600)

	clientKey := SFTPClient{
		Host:                    "127.0.0.1",
		Port:                    port,
		User:                    "user1",
		KeyPath:                 keyPath,
		SkipHostKeyVerification: true,
	}
	if err := clientKey.Connect(); err != nil {
		t.Fatalf("Expected connect with key to succeed: %v", err)
	}
	clientKey.Close()
}

func TestSFTPClientConnect_MockErrors(t *testing.T) {
	client := &SFTPClient{KeyPath: "somepath", User: "u", Host: "h", Port: "p", SkipHostKeyVerification: true}

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
	clientNoAuth := &SFTPClient{User: "u", Host: "h", Port: "p", SkipHostKeyVerification: true}
	if err := clientNoAuth.Connect(); err == nil || !strings.Contains(err.Error(), "no authentication methods") {
		t.Errorf("Expected no auth methods error, got %v", err)
	}

	// 4. KnownHostsPath error
	clientKH := &SFTPClient{User: "u", Host: "h", Port: "p", KnownHostsPath: "invalid", Password: "p"}
	if err := clientKH.Connect(); err == nil || !strings.Contains(err.Error(), "failed to load known hosts") {
		t.Errorf("Expected known hosts error, got %v", err)
	}
	
	// 5. ssh.Dial error
	clientDial := &SFTPClient{Host: "invalid", Port: "22", User: "u", Password: "p", SkipHostKeyVerification: true}
	if err := clientDial.Connect(); err == nil {
		t.Error("Expected error for invalid dial")
	}
}

func TestSFTPClient_FullCoverage(t *testing.T) {
	mockFile := &mockSFTPFile{statSize: 10}
	mockC := &mockSFTPClient{
		createFunc: func(path string) (SFTPFile, error) { return mockFile, nil },
		statFunc:   func(path string) (os.FileInfo, error) { return &mockFileInfo{size: 10}, nil },
	}
	client := &SFTPClient{
		RemoteDir:  ".",
		sftpClient: mockC,
		Overwrite:  true,
	}

	tempDir, _ := os.MkdirTemp("", "full_cov")
	defer os.RemoveAll(tempDir)
	testFile := filepath.Join(tempDir, "test.txt")
	os.WriteFile(testFile, []byte("1234567890"), 0644)

	// 1. Success
	if err := client.UploadFile(context.Background(), testFile); err != nil {
		t.Errorf("Expected success, got %v", err)
	}

	// 2. Stat local fail
	oldOpen := osOpen
	osOpen = func(name string) (File, error) {
		f, err := os.Open(name)
		if err != nil {
			return nil, err
		}
		return &mockFileObj{osFile: f, statErr: fmt.Errorf("stat fail")}, nil
	}
	if err := client.UploadFile(context.Background(), testFile); err == nil || !strings.Contains(err.Error(), "stat fail") {
		t.Errorf("Expected stat fail, got %v", err)
	}
	osOpen = oldOpen

	// 3. Create remote fail
	oldCreate := mockC.createFunc
	mockC.createFunc = func(path string) (SFTPFile, error) { return nil, fmt.Errorf("create fail") }
	if err := client.UploadFile(context.Background(), testFile); err == nil || !strings.Contains(err.Error(), "create fail") {
		t.Errorf("Expected create fail, got %v", err)
	}
	mockC.createFunc = oldCreate

	// 4. io.Copy fail
	mockFile.writeErr = fmt.Errorf("write fail")
	if err := client.UploadFile(context.Background(), testFile); err == nil || !strings.Contains(err.Error(), "write fail") {
		t.Errorf("Expected write fail, got %v", err)
	}
	mockFile.writeErr = nil

	// 4b. remote close fail
	mockFile.closeErr = fmt.Errorf("remote close fail")
	if err := client.UploadFile(context.Background(), testFile); err == nil || !strings.Contains(err.Error(), "remote close fail") {
		t.Errorf("Expected remote close fail, got %v", err)
	}
	mockFile.closeErr = nil

	// 5. Remote Stat fail (verification)
	oldStat := mockC.statFunc
	mockC.statFunc = func(path string) (os.FileInfo, error) { return nil, fmt.Errorf("stat remote fail") }
	if err := client.UploadFile(context.Background(), testFile); err == nil || !strings.Contains(err.Error(), "stat remote fail") {
		t.Errorf("Expected stat remote fail, got %v", err)
	}
	mockC.statFunc = oldStat

	// 6. Size mismatch
	mockC.statFunc = func(path string) (os.FileInfo, error) { return &mockFileInfo{size: 5}, nil }
	if err := client.UploadFile(context.Background(), testFile); err == nil || !strings.Contains(err.Error(), "size mismatch") {
		t.Errorf("Expected size mismatch, got %v", err)
	}
	mockC.statFunc = oldStat

	// 7. Delete fail coverage
	client.DeleteAfterVerify = true
	oldRemove := osRemove
	osRemove = func(name string) error { return fmt.Errorf("remove error") }
	if err := client.UploadFile(context.Background(), testFile); err != nil {
		t.Errorf("Expected success even if remove fails, got %v", err)
	}
	osRemove = oldRemove

	// 8. Delete success coverage
	if err := client.UploadFile(context.Background(), testFile); err != nil {
		t.Errorf("Expected success on delete, got %v", err)
	}

	// 9. UploadFileWithRetry failure path
	oldOpen2 := osOpen
	osOpen = func(name string) (File, error) { return nil, fmt.Errorf("open fail") }
	if err := client.UploadFileWithRetry(context.Background(), testFile, 2); err == nil || !strings.Contains(err.Error(), "upload failed after 2 attempts") {
		t.Errorf("Expected retry fail, got %v", err)
	}
	osOpen = oldOpen2
}

func TestSFTPClientUpload_AdvancedNetwork(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "adv_network")
	defer os.RemoveAll(tempDir)
	testFile := filepath.Join(tempDir, "test.txt")
	os.WriteFile(testFile, []byte("large data chunk for testing network issues"), 0644)

	mockC := &mockSFTPClient{}
	client := &SFTPClient{
		RemoteDir:  ".",
		sftpClient: mockC,
		Overwrite:  true,
	}

	// 1. Test Interrupted Transfer (Failure during Write)
	mockC.createFunc = func(path string) (SFTPFile, error) {
		return &mockSFTPFile{statSize: 43, failAt: 1}, nil
	}
	mockC.statFunc = func(path string) (os.FileInfo, error) {
		return &mockFileInfo{size: 43}, nil
	}
	if err := client.UploadFile(context.Background(), testFile); err == nil || !strings.Contains(err.Error(), "interrupted transfer") {
		t.Errorf("Expected interrupted transfer error, got %v", err)
	}

	// 2. Test Cancellation via context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancel() // Cancel immediately
	if err := client.UploadFile(ctx, testFile); err == nil || (!strings.Contains(err.Error(), "context canceled") && !strings.Contains(err.Error(), "operation was canceled")) {
		t.Errorf("Expected context canceled error, got %v", err)
	}

	err := client.UploadFile(context.Background(), testFile)
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

	if err := client.UploadFileWithRetry(context.Background(), testFile, 2); err != nil {
		t.Errorf("Expected retry to succeed on second attempt, got %v", err)
	}

	// 3. Test High Latency
	mockC.createFunc = func(path string) (SFTPFile, error) {
		return &mockSFTPFile{statSize: 43, delay: 10 * time.Millisecond}, nil
	}

	if err := client.UploadFile(context.Background(), testFile); err != nil {
		t.Errorf("Expected success with latency, got %v", err)
	}
}

func TestSFTPClient_RateLimiting(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "ratelimit_test")
	defer os.RemoveAll(tempDir)
	testFile := filepath.Join(tempDir, "test.txt")
	// 60KB of data
	data := make([]byte, 60*1024)
	os.WriteFile(testFile, data, 0644)

	mockFile := &mockSFTPFile{statSize: 60 * 1024}
	mockC := &mockSFTPClient{
		createFunc: func(path string) (SFTPFile, error) { return mockFile, nil },
		statFunc:   func(path string) (os.FileInfo, error) { return &mockFileInfo{size: 60 * 1024}, nil },
	}

	// Test Fixed Rate Limit: 10KB/s. 60KB should take ~6s.
	client := &SFTPClient{
		RemoteDir:     ".",
		sftpClient:    mockC,
		RateLimitKBps: 10,
		Overwrite:     true,
	}

	start := time.Now()
	if err := client.UploadFile(context.Background(), testFile); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	duration := time.Since(start)
	if duration < 3000*time.Millisecond {
		t.Errorf("Expected delay for 60KB at 10KB/s, took %v", duration)
	}

	mockFile.delay = 50 * time.Millisecond
	clientDynamic := &SFTPClient{
		RemoteDir:    ".",
		sftpClient:   mockC,
		MaxLatencyMs: 10,
		Overwrite:    true,
	}

	start = time.Now()
	if err := clientDynamic.UploadFile(context.Background(), testFile); err != nil {
		t.Fatalf("Dynamic upload failed: %v", err)
	}
}

func TestSFTPClient_ValidateRemotePath(t *testing.T) {
	client := &SFTPClient{RemoteDir: "/upload", sftpClient: &mockSFTPClient{}}
	
	p, err := client.validateRemotePath("/upload/file.txt")
	if err != nil { t.Errorf("Expected success, got %v", err) }
	if p != "/upload/file.txt" { t.Errorf("Unexpected path: %s", p) }

	_, err = client.validateRemotePath("/upload/../etc/passwd")
	if err == nil || !strings.Contains(err.Error(), "traversal detected") {
		t.Errorf("Expected traversal error, got %v", err)
	}

	_, err = client.validateRemotePath("/etc/passwd")
	if err == nil || !strings.Contains(err.Error(), "traversal detected") {
		t.Errorf("Expected traversal error for path outside base, got %v", err)
	}
}

func TestSFTPClient_FileSystemActions(t *testing.T) {
	mockC := &mockSFTPClient{}
	client := &SFTPClient{RemoteDir: "/upload", sftpClient: mockC}

	if err := client.Mkdir("/upload/new"); err != nil { t.Error(err) }
	if err := client.Remove("/upload/old"); err != nil { t.Error(err) }
	if err := client.Rename("/upload/old", "/upload/new"); err != nil { t.Error(err) }

	if err := client.Mkdir("/root"); err == nil { t.Error("Expected traversal error") }
	if err := client.Remove("/root"); err == nil { t.Error("Expected traversal error") }
	if err := client.Rename("/upload/a", "/root"); err == nil { t.Error("Expected traversal error") }
}

func TestSFTPClient_ReadRemoteDir(t *testing.T) {
	mockC := &mockSFTPClient{}
	client := &SFTPClient{RemoteDir: "/upload", sftpClient: mockC}

	mockC.readDirFunc = func(p string) ([]os.FileInfo, error) {
		return []os.FileInfo{&mockFileInfo{name: "f1", size: 100}}, nil
	}
	files, err := client.ReadRemoteDir("/upload")
	if err != nil { t.Fatal(err) }
	if len(files) != 1 { t.Errorf("Expected 1 file, got %d", len(files)) }

	_, err = client.ReadRemoteDir("/etc")
	if err == nil { t.Error("Expected traversal error") }
}

func TestSFTPClient_OverwriteCheckErrors(t *testing.T) {
	mockC := &mockSFTPClient{}
	client := &SFTPClient{RemoteDir: "/upload", sftpClient: mockC, Overwrite: false}

	tempDir, _ := os.MkdirTemp("", "overwrite_test")
	defer os.RemoveAll(tempDir)
	testFile := filepath.Join(tempDir, "test.txt")
	os.WriteFile(testFile, []byte("data"), 0644)

	mockC.statFunc = func(path string) (os.FileInfo, error) {
		return nil, fmt.Errorf("permission denied")
	}
	err := client.UploadFile(context.Background(), testFile)
	if err == nil || !strings.Contains(err.Error(), "failed to check remote file existence") {
		t.Errorf("Expected existence check error, got %v", err)
	}
}

func TestThrottledReader_LargeRead(t *testing.T) {
	limiter := NewLimiter(1024, 1024, 0)
	tr := &throttledReader{
		ctx:     context.Background(),
		r:       strings.NewReader(strings.Repeat("a", 2048)),
		limiter: limiter,
	}
	p := make([]byte, 2048)
	n, err := tr.Read(p)
	if err != nil { t.Fatal(err) }
	if n != 2048 { t.Errorf("Expected 2048 bytes, got %d", n) }
}

func TestThrottledWriter_LargeWrite(t *testing.T) {
	limiter := NewLimiter(1024, 1024, 0)
	tw := &throttledWriter{
		ctx:     context.Background(),
		w:       io.Discard,
		limiter: limiter,
	}
	n, err := tw.Write([]byte(strings.Repeat("a", 2048)))
	if err != nil { t.Fatal(err) }
	if n != 2048 { t.Errorf("Expected 2048 bytes, got %d", n) }
}

func TestThrottledReader_WaitError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancel()
	limiter := NewLimiter(1024, 1024, 0)
	tr := &throttledReader{
		ctx:     ctx,
		r:       strings.NewReader("any"),
		limiter: limiter,
	}
	p := make([]byte, 3)
	_, err := tr.Read(p)
	if err == nil { t.Error("Expected context error") }
}

func TestThrottledWriter_WaitError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancel()
	limiter := NewLimiter(1024, 1024, 0)
	tw := &throttledWriter{
		ctx:     ctx,
		w:       io.Discard,
		limiter: limiter,
	}
	_, err := tw.Write([]byte("any"))
	if err == nil { t.Error("Expected context error") }
}

func TestThrottledWriter_InfLimit(t *testing.T) {
	tw := &throttledWriter{
		ctx:     context.Background(),
		w:       io.Discard,
		limiter: NewLimiter(rate.Inf, 0, 0),
		maxLatency: time.Millisecond,
	}
	_, err := tw.Write([]byte("any"))
	if err != nil { t.Error(err) }
}

func TestSFTPClient_ConnectFailuresExhaustive(t *testing.T) {
	client := &SFTPClient{
		Host: "localhost", Port: "22", User: "u", 
		KeyPath: "invalid", KnownHostsPath: "invalid",
	}

	oldRead := osReadFile
	osReadFile = func(name string) ([]byte, error) { return nil, os.ErrPermission }
	if err := client.Connect(); err == nil { t.Error("Expected error for osReadFile") }
	osReadFile = oldRead

	client.KeyPath = ""
	if err := client.Connect(); err == nil { t.Error("Expected error for knownhosts.New") }

	client.KnownHostsPath = ""
	client.SkipHostKeyVerification = true
	client.Password = "p"
	if err := client.Connect(); err == nil { t.Error("Expected error for ssh.Dial") }
}

func TestSFTPClient_UploadFailuresExhaustive(t *testing.T) {
	t.Run("osOpen Error", func(t *testing.T) {
		mockC := &mockSFTPClient{}
		client := &SFTPClient{RemoteDir: "/upload", sftpClient: mockC, Overwrite: true}
		tempDir, _ := os.MkdirTemp("", "osopen_err")
		defer os.RemoveAll(tempDir)
		testFile := filepath.Join(tempDir, "test.txt")
		os.WriteFile(testFile, []byte("data"), 0644)

		oldOpen := osOpen
		osOpen = func(name string) (File, error) { return nil, os.ErrPermission }
		defer func() { osOpen = oldOpen }()
		if err := client.UploadFile(context.Background(), testFile); err == nil { t.Error("Expected error for osOpen") }
	})

	t.Run("sftpClient.Create Error", func(t *testing.T) {
		mockC := &mockSFTPClient{}
		client := &SFTPClient{RemoteDir: "/upload", sftpClient: mockC, Overwrite: true}
		tempDir, _ := os.MkdirTemp("", "create_err")
		defer os.RemoveAll(tempDir)
		testFile := filepath.Join(tempDir, "test.txt")
		os.WriteFile(testFile, []byte("data"), 0644)

		mockC.createFunc = func(path string) (SFTPFile, error) { return nil, os.ErrPermission }
		if err := client.UploadFile(context.Background(), testFile); err == nil { t.Error("Expected error for sftpClient.Create") }
	})

	t.Run("Cleanup Error", func(t *testing.T) {
		mockC := &mockSFTPClient{}
		client := &SFTPClient{RemoteDir: "/upload", sftpClient: mockC, Overwrite: true}
		tempDir, _ := os.MkdirTemp("", "cleanup_err")
		defer os.RemoveAll(tempDir)
		testFile := filepath.Join(tempDir, "test.txt")
		os.WriteFile(testFile, []byte("data"), 0644)

		mockC.createFunc = func(path string) (SFTPFile, error) {
			return &mockSFTPFile{writeErr: fmt.Errorf("write fail")}, nil
		}
		mockC.removeErr = fmt.Errorf("remove fail")
		_ = client.UploadFile(context.Background(), testFile)
	})

	t.Run("osRemove Error", func(t *testing.T) {
		mockC := &mockSFTPClient{}
		client := &SFTPClient{RemoteDir: "/upload", sftpClient: mockC, Overwrite: true, DeleteAfterVerify: true}
		tempDir, _ := os.MkdirTemp("", "osremove_err")
		defer os.RemoveAll(tempDir)
		testFile := filepath.Join(tempDir, "test.txt")
		os.WriteFile(testFile, []byte("data"), 0644)

		mockC.createFunc = func(path string) (SFTPFile, error) { return &mockSFTPFile{statSize: 4}, nil }
		mockC.statFunc = func(path string) (os.FileInfo, error) { return &mockFileInfo{size: 4}, nil }
		oldRemove := osRemove
		osRemove = func(name string) error { return os.ErrPermission }
		defer func() { osRemove = oldRemove }()
		if err := client.UploadFile(context.Background(), testFile); err != nil { t.Error(err) }
	})
}

func TestSFTPClient_UploadRetryFailureExhaustive(t *testing.T) {
	mockC := &mockSFTPClient{}
	client := &SFTPClient{RemoteDir: "/upload", sftpClient: mockC, Overwrite: true}
	tempDir, _ := os.MkdirTemp("", "retry_fail_test")
	defer os.RemoveAll(tempDir)
	testFile := filepath.Join(tempDir, "test.txt")
	os.WriteFile(testFile, []byte("data"), 0644)

	mockC.createFunc = func(path string) (SFTPFile, error) { return nil, fmt.Errorf("fail") }
	if err := client.UploadFileWithRetry(context.Background(), testFile, 1); err == nil {
		t.Error("Expected error for UploadFileWithRetry")
	}
}

func TestSFTPClient_UploadResumeMismatch(t *testing.T) {
	tempDir, _ := os.MkdirTemp("", "resume_mismatch")
	defer os.RemoveAll(tempDir)
	testFile := filepath.Join(tempDir, "test.txt")
	// Local file
	localContent := []byte("NEW CONTENT")
	os.WriteFile(testFile, localContent, 0644)

	// Remote partial file has different content
	remoteContent := []byte("OLD CONTENT")
	mockFile := &mockSFTPFile{
		content:  remoteContent,
		statSize: int64(len(remoteContent)),
	}

	createdNew := false
	mockC := &mockSFTPClient{
		statFunc: func(path string) (os.FileInfo, error) {
			if strings.HasSuffix(path, ".tmp") {
				return &mockFileInfo{size: int64(len(remoteContent))}, nil
			}
			return nil, os.ErrNotExist
		},
		openFileFunc: func(path string, flags int) (SFTPFile, error) {
			return mockFile, nil
		},
		createFunc: func(path string) (SFTPFile, error) {
			createdNew = true
			return &mockSFTPFile{}, nil
		},
	}

	client := &SFTPClient{
		RemoteDir:  ".",
		sftpClient: mockC,
		Overwrite:  true,
	}

	if err := client.UploadFile(context.Background(), testFile); err != nil {
		t.Errorf("Expected success after mismatch restart, got %v", err)
	}

	if !createdNew {
		t.Error("Expected SFTPClient to call Create() after content mismatch, but it didn't")
	}
}

