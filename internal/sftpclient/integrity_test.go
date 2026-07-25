package sftpclient

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func sha256File(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path) // #nosec G304 -- test fixture path
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestUploadIntegrityAcrossPacketSizes uploads through a real SFTP server and
// compares hashes, not just sizes.
//
// Concurrent writes dispatch many out-of-order writes at explicit offsets, and
// the upload path only verifies the final size. A packet-boundary or offset bug
// would produce a file of exactly the right length with the wrong bytes, which
// size verification cannot catch. Each packet size is exercised because the
// payload size determines those boundaries.
func TestUploadIntegrityAcrossPacketSizes(t *testing.T) {
	packetSizes := []string{"32768", "65536", "131072"}

	for _, size := range packetSizes {
		t.Run("packet="+size, func(t *testing.T) {
			t.Setenv("UPLARR_SFTP_MAX_PACKET", size)

			localDir := t.TempDir()
			remoteDir := t.TempDir()

			// Several packets' worth, with a deliberately non-aligned tail so the
			// final short write is covered too.
			payload := make([]byte, 5*1024*1024+1234)
			if _, err := rand.Read(payload); err != nil {
				t.Fatalf("generate payload: %v", err)
			}
			srcPath := filepath.Join(localDir, "payload.bin")
			if err := os.WriteFile(srcPath, payload, 0600); err != nil {
				t.Fatalf("write source: %v", err)
			}

			port, cleanup := startMockSFTPServer(t, "user1", "pass1", remoteDir)
			defer cleanup()

			client := SFTPClient{
				Host:                    "127.0.0.1",
				Port:                    port,
				User:                    "user1",
				Password:                "pass1",
				SkipHostKeyVerification: true,
				// The mock server resolves relative paths against remoteDir. An
				// absolute Windows path is not a valid POSIX SFTP path and would
				// be joined onto the server's working directory.
				RemoteDir: ".",
				Overwrite: true,
			}
			if err := client.Connect(); err != nil {
				t.Fatalf("connect: %v", err)
			}
			defer client.Close()

			if err := client.UploadFile(context.Background(), srcPath); err != nil {
				t.Fatalf("upload: %v", err)
			}

			// Landing under the final name matters on its own: the upload closes
			// its remote handle before renaming, and a Windows-hosted SFTP server
			// refuses to rename a file that still has one open. Asserting the
			// final name (never the .tmp) keeps that ordering from regressing.
			dstPath := filepath.Join(remoteDir, "payload.bin")
			info, err := os.Stat(dstPath)
			if err != nil {
				t.Fatalf("uploaded file missing at final name: %v", err)
			}
			if _, err := os.Stat(dstPath + ".tmp"); err == nil {
				t.Error("temp file still present; rename did not replace it")
			}
			if info.Size() != int64(len(payload)) {
				t.Fatalf("size = %d, want %d", info.Size(), len(payload))
			}

			if got, want := sha256File(t, dstPath), sha256File(t, srcPath); got != want {
				t.Errorf("content hash mismatch at packet size %s:\n got %s\nwant %s", size, got, want)
			}
		})
	}
}
