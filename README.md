# Uplarr

Uplarr is a zero-bloat, production-ready Go application with a built-in Web GUI for viewing local files and triggering uploads to a remote SFTP server with verification.

## Features
- **Local File Viewer**: View files in a mounted local directory.
- **SFTP Upload**: Securely upload files to a remote server.
- **Verification**: Verifies remote file size against local file size.
- **Cleanup**: Optionally delete local files after successful verification.
- **Zero Bloat**: No external frontend dependencies, minimal backend dependencies.
- **Dockerized**: Multi-stage, multi-architecture (x86/ARM) Docker support.

## Configuration (Environment Variables)
- `SFTP_HOST`: SFTP server hostname (default: `localhost`).
- `SFTP_PORT`: SFTP server port (default: `22`).
- `SFTP_USER`: SFTP username (default: `user`).
- `SFTP_PASSWORD`: SFTP password (default: `password`).
- `SFTP_KEY_PATH`: Path to private SSH key (optional).
- `LOCAL_DIR`: Local directory to monitor (default: `./test_data`).
- `REMOTE_DIR`: Remote directory on SFTP server (default: `/upload`).
- `DELETE_AFTER_VERIFY`: If `true`, deletes local files after successful upload (default: `false`).
- `WEB_PORT`: Port for the Web GUI (default: `8080`).

## Local Development
1. Clone the repository.
2. Install dependencies: `go mod download`.
3. Run the app: `go run .`.
4. Open `http://localhost:8080`.

## Testing with Docker Compose
To test the integration locally with a mock SFTP server:
```bash
mkdir -p test_data
echo "Hello World" > test_data/test.txt
docker-compose up --build
```
Access the UI at `http://localhost:8080`.

## Multi-Architecture Build
To build and push the image for both x86 and ARM:
```bash
# 1. Create a new builder
docker buildx create --use

# 2. Build and push the image
docker buildx build --platform linux/amd64,linux/arm64 -t your-username/uplarr:latest --push .
```

## Tech Stack
- **Backend**: Go (standard library + `golang.org/x/crypto/ssh`, `github.com/pkg/sftp`).
- **Frontend**: Vanilla HTML5, CSS3, and JavaScript (embedded in binary).
- **Docker**: Alpine-based minimal image.
