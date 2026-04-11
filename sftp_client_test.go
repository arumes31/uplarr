package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
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
		PublicKeyCallback: func(c ssh.ConnMetadata, pubKey ssh.PublicKey) (*ssh.Permissions, error) {
			if c.User() == user {
				return nil, nil // accept any key for the test user
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

	// Test No auth methods
	clientNoAuth := SFTPClient{
		Host: "127.0.0.1",
		Port: port,
		User: "user1",
	}
	if err := clientNoAuth.Connect(); err == nil {
		t.Fatal("Expected connect to fail with no auth methods")
	}

	// Test Public Key auth
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

	// Test invalid key path
	clientKeyInvalid := SFTPClient{
		Host:    "127.0.0.1",
		Port:    port,
		User:    "user1",
		KeyPath: "nonexistent",
	}
	if err := clientKeyInvalid.Connect(); err == nil {
		t.Fatal("Expected connect to fail with invalid key path")
	}

	// Test invalid key file content
	invalidKeyPath := filepath.Join(tempDir, "id_rsa_invalid")
	os.WriteFile(invalidKeyPath, []byte("invalid"), 0600)
	clientKeyInvalidContent := SFTPClient{
		Host:    "127.0.0.1",
		Port:    port,
		User:    "user1",
		KeyPath: invalidKeyPath,
	}
	if err := clientKeyInvalidContent.Connect(); err == nil {
		t.Fatal("Expected connect to fail with invalid key content")
	}

	// Test dial error
	clientDialErr := SFTPClient{
		Host:     "127.0.0.1",
		Port:     "0",
		User:     "user1",
		Password: "p",
	}
	if err := clientDialErr.Connect(); err == nil {
		t.Fatal("Expected connect to fail with dial error")
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

	// Test successful upload
	testFile := filepath.Join(localDir, "test.txt")
	os.WriteFile(testFile, []byte("hello world"), 0644)

	if err := client.UploadFile(testFile); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	// Verify it was uploaded
	remoteFile := filepath.Join(remoteDir, "test.txt")
	if content, _ := os.ReadFile(remoteFile); string(content) != "hello world" {
		t.Fatalf("Expected file content 'hello world', got '%s'", string(content))
	}

	// Test open local file error
	if err := client.UploadFile(filepath.Join(localDir, "nonexistent.txt")); err == nil {
		t.Fatal("Expected upload to fail with nonexistent local file")
	}

	// Test DeleteAfterVerify
	client.DeleteAfterVerify = true
	testFile2 := filepath.Join(localDir, "test2.txt")
	os.WriteFile(testFile2, []byte("delete me"), 0644)
	if err := client.UploadFile(testFile2); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}
	if _, err := os.Stat(testFile2); !os.IsNotExist(err) {
		t.Fatal("Expected local file to be deleted")
	}

	// Test DeleteAfterVerify failure handling (cannot delete file due to permissions/locks)
	// We can create a file, upload it, but mock a failure to delete by simulating a removed directory
	// This is hard to do cleanly without breaking other tests. Skipping detailed error cover on delete for now.

	// Test UploadFileWithRetry
	testFile3 := filepath.Join(localDir, "test3.txt")
	os.WriteFile(testFile3, []byte("retry me"), 0644)
	if err := client.UploadFileWithRetry(testFile3, 3); err != nil {
		t.Fatalf("UploadWithRetry failed: %v", err)
	}

	// Test UploadFileWithRetry failure (remote directory doesn't exist)
	clientInvalidRemote := SFTPClient{
		Host:      "127.0.0.1",
		Port:      port,
		User:      "user1",
		Password:  "pass1",
		RemoteDir: "/invalid_dir_that_causes_failure",
	}
	clientInvalidRemote.Connect()
	defer clientInvalidRemote.Close()

	if err := clientInvalidRemote.UploadFileWithRetry(testFile3, 2); err == nil {
		t.Fatal("Expected UploadWithRetry to fail after retries")
	}

	// Test remote verification failure
	// We mock this by replacing the file on the remote side between copy and verify.
	// We can't easily hook into the middle of the function, but we can verify the code coverage.
}
