package sftpclient

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadRootFileWithRetry(t *testing.T) {
	mockC := &mockSFTPClient{
		createFunc: func(path string) (SFTPFile, error) { return &mockSFTPFile{statSize: 10}, nil },
		statFunc:   func(path string) (os.FileInfo, error) { return &mockFileInfo{size: 10}, nil },
	}
	client := &SFTPClient{RemoteDir: ".", sftpClient: mockC, Overwrite: true, DeleteAfterVerify: true}

	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, "ok.txt"), []byte("1234567890"), 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })

	if err := client.UploadRootFileWithRetry(context.Background(), root, "ok.txt", 1); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "ok.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected delete-after-verify to remove the file through the root, stat err = %v", err)
	}
}

// A file swapped for a symlink that escapes the root after validation must
// not be read by the upload.
func TestUploadRootFileWithRetry_RejectsEscapingSymlink(t *testing.T) {
	created := false
	mockC := &mockSFTPClient{
		createFunc: func(path string) (SFTPFile, error) {
			created = true
			return &mockSFTPFile{statSize: 6}, nil
		},
		statFunc: func(path string) (os.FileInfo, error) { return &mockFileInfo{size: 6}, nil },
	}
	client := &SFTPClient{RemoteDir: ".", sftpClient: mockC, Overwrite: true}

	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	base := t.TempDir()
	if err := os.Symlink(secret, filepath.Join(base, "swapped.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	root, err := os.OpenRoot(base)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })

	if err := client.UploadRootFileWithRetry(context.Background(), root, "swapped.txt", 1); err == nil {
		t.Fatal("expected upload through an escaping symlink to fail")
	}
	if created {
		t.Fatal("remote file was created for an escaping symlink")
	}
}
