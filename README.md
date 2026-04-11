# Uplarr

Uplarr is a zero-bloat, production-ready Go application with a modern Web GUI for viewing local files and triggering uploads to remote SFTP servers with verification.

## Features
- **Dynamic SFTP Connections**: Configure and test SFTP connections directly in the web GUI.
- **Local File Viewer**: View files in a mounted local directory.
- **SFTP Upload with Verification**: Securely upload files and verify remote file integrity via size comparison.
- **Auto-Cleanup**: Optional deletion of local files after successful verification.
- **Modern UI**: Clean, responsive interface with real-time process logs.
- **Containerized**: Minimal, multi-stage Docker image with built-in testing.

## Configuration (Environment Variables)
- `LOCAL_DIR`: Local directory to monitor (default: `./test_data`).
- `WEB_PORT`: Port for the Web GUI (default: `8080`).

*Note: SFTP host, user, and credentials are now configured dynamically via the Web GUI.*

## Local Development
1. Clone the repository.
2. Install dependencies: `go mod download`.
3. Run the app: `go run .`.
4. Open `http://localhost:8080`.

## Testing with Docker
You can run all tests within a Docker container to ensure environment parity:
```bash
docker build -t uplarr-test --target builder .
```

To run the full application:
```bash
docker build -t uplarr .
docker run -p 8080:8080 -v /path/to/local/data:/root/test_data uplarr
```

## Tech Stack
- **Backend**: Go (standard library + `golang.org/x/crypto/ssh`, `github.com/pkg/sftp`).
- **Frontend**: Vanilla HTML5, modern CSS3, and JavaScript (embedded).
- **Docker**: Alpine-based minimal image with multi-stage build.

## Improvements Made
- **Dynamic SFTP**: Removed static environment variables for SFTP credentials; now handled via UI.
- **Connection Testing**: Added `/api/test-connection` to verify settings before starting uploads.
- **Robust Error Handling**: Improved API responses to provide detailed feedback on partial failures.
- **Polished UI**: Enhanced visual design and user feedback.
- **Test Coverage**: Expanded test suite to cover new endpoints and edge cases.
