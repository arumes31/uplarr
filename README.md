<p align="center"><img src="ui/static/logo.png" alt="Uplarr Logo" width="200"></p>

# Uplarr - GO SFTP Web Client


<p align="center">
  <img src="https://img.shields.io/badge/Go-stable-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version">
  <img src="https://img.shields.io/badge/Docker-Multi--Arch-2496ED?style=for-the-badge&logo=docker&logoColor=white" alt="Docker">
  <img src="https://img.shields.io/github/actions/workflow/status/arumes31/uplarr/ci.yml?branch=main&style=for-the-badge&logo=github-actions&label=CI" alt="CI Status">
  <img src="https://img.shields.io/github/v/release/arumes31/uplarr?style=for-the-badge&logo=github&color=blue" alt="Latest Release">
  <img src="https://img.shields.io/badge/go%20report-A+-brightgreen.svg?style=for-the-badge" alt="Go Report Card">
  <img src="https://goreportcard.com/badge/github.com/arumes31/uplarr" alt="Go Report Card (Real)">
  <img src="https://img.shields.io/badge/Security-Gosec-brightgreen?style=for-the-badge&logo=shippable" alt="Security Scan">
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="License">
</p>

**Uplarr** is a high-performance, zero-bloat Go application designed to bridge the gap between local storage and remote SFTP servers. With a sleek modern Web GUI, real-time progress logging via SSE, and robust verification logic, Uplarr ensures your data moves safely and efficiently.

---

## 📊 System Architecture

```mermaid
graph TD
    User([User]) <--> WebGUI[Modern Web GUI - Vanilla JS/CSS]
    WebGUI <--> API[Go HTTP API]
    
    subgraph Backend [Go Backend - 1.26]
        API <--> Broadcaster[SSE Log Broadcaster]
        API <--> Queue[File Queue Manager]
        Queue --> SFTP[SFTP Client - golang.org/x/crypto/ssh]
        SFTP <--> Limiter[Rate Limiter - golang.org/x/time/rate]
    end
    
    subgraph Storage
        Local[(Local Storage /test_data)]
        Remote[(Remote SFTP Server)]
    end
    
    Queue <--> Local
    SFTP <--> Remote
    Broadcaster -- Push Updates --> User
```

---

## ✨ Key Features

- 📦 **Background Queue**: Persistent task manager that survives container restarts, with full queue management and pause/resume support.
- 📁 **File Management**: Create folders, rename, and delete files on both local and remote filesystems.
- 📂 **WinSCP-Style Browser**: Advanced dual-pane interface for browsing local and remote files with full directory navigation.
- 🔄 **Mass Rename Utility**: Powerful regex-based bulk renaming with real-time preview and sequence formatting (`$idx`).
- 🖱️ **Drag & Drop**: Seamlessly upload files by dragging them from the local pane to the remote directory of your choice.
- 🛠 **Dynamic Configuration**: Configure and test SFTP connections, including host key verification toggles and advanced latency floor tuning.
- 📉 **Compact View Mode**: Toggleable high-density interface for managing large file structures with horizontal layout optimization.
- 📡 **Real-time SSE Logs**: Integrated Server-Sent Events (SSE) provide live terminal-style feedback for all operations.
- 🔐 **Encrypted Storage**: All persistent settings (including credentials) are AES-GCM encrypted in the browser with secure master-key management.
- 🤖 **Automated CI/CD**: Pushes to `main` trigger automated semantic versioning and cross-platform builds.
- 🐳 **Enterprise Ready**: Multi-arch Docker images (`amd64`, `arm64`) and automated security scanning via GitHub Container Registry.

---

## 📸 Interface Preview

> [!TIP]
> The interface is designed to be fully responsive and works beautifully on mobile or desktop.

| Feature | Description |
| :--- | :--- |
| **File Browser** | Interactive list with checkboxes for batch queuing. |
| **Config Panel** | Secure form for credential and host management. |
| **Log Terminal** | Real-time scrollable window for process auditing. |

---

## 🛠 Quick Start

### Using Docker (Recommended)

Uplarr now provides automated multi-arch builds (`amd64`, `arm64`) via the GitHub Container Registry.

```bash
docker pull ghcr.io/arumes31/uplarr:latest

docker run -d \
  -p 8080:8080 \
  -v /your/local/data:/root/test_data \
  --name uplarr \
  ghcr.io/arumes31/uplarr:latest
```

### Using Docker Compose

For a complete production-ready setup using the GitHub Container Registry image, see the [docker-compose.ghcr.yml](docker-compose.ghcr.yml) example.
The project is configured to automatically increment versions and push images to GHCR on every push to `main`.

Quick setup: `docker compose -f docker-compose.ghcr.yml up -d`

Or create a manual `docker-compose.yml`:

```yaml
services:
  uplarr:
    image: ghcr.io/arumes31/uplarr:main
    ports:
      - "8080:8080"
    environment:
      - LOCAL_DIR=/data
      - CONFIG_DIR=/config
      - AUTH_PASSWORD=your_secure_password
    volumes:
      - ./config:/config:rw
      - /path/to/local/data:/data:ro
```


Run with: `docker compose up -d`

### Local Development

1. **Prerequisites**: Go 1.26+ installed.
2. **Install & Run**:
   ```bash
   go mod download
   go run .
   ```
3. **Access**: Open [http://localhost:8080](http://localhost:8080).

---

## ⚙️ Configuration (Environment Variables)

| Variable | Description | Default |
| :--- | :--- | :--- |
| `LOCAL_DIR` | Directory to monitor for files | `./test_data` |
| `CONFIG_DIR` | Directory for application state (queue) | `./config` |
| `WEB_PORT` | Port for the Web GUI | `8080` |
| `AUTH_PASSWORD` | Password for Web UI authentication | (None) |
| `UPLARR_SFTP_MAX_PACKET` | SFTP payload bytes per request (1024–131072) | `32768` |
| `UPLARR_SFTP_MAX_REQUESTS` | Concurrent in-flight requests per file (1–1024) | `128` |

*Connection details (host, credentials, remote path) are managed dynamically via
the Web UI. The two `UPLARR_SFTP_*` variables tune transport throughput — see
[SFTP Tuning](#-sftp-tuning) below.*

---

## 💾 Storage & Persistence

Uplarr maintains a background queue that survives container and process restarts.

- **State File**: `.queue_state.json`
- **Location**: Stored in your configured `CONFIG_DIR` (defaults to `./config`).
- **Isolation**: Keeping state in a separate directory allows you to mount `LOCAL_DIR` as **read-only** (`:ro`), improving overall security for your media files.
- **Persistence**: The `CONFIG_DIR` mount **must be writable** (`:rw`) for the application to save its queue.

---

## 🔧 SFTP Tuning

Uploads pipeline many requests at once, so a single large file is not limited
to one packet per network round trip. Both knobs below are environment
variables — no rebuild needed — and the values actually in use are written to
the log on every connection.

### `UPLARR_SFTP_MAX_PACKET` (default `32768`)

Payload bytes per request, and the single biggest throughput lever. Against an
in-memory test server, raising this from `32768` to `131072` roughly doubled
single-file throughput.

`32768` is the default because it is the only payload size the SFTP
specification requires every server to accept. Raise it only against a server
you control:

```yaml
environment:
  - UPLARR_SFTP_MAX_PACKET=131072
```

Values above `131072` are rejected, because the packet header shares SFTP's
256 KiB message limit and an oversized packet makes the server drop the
connection mid-transfer.

### `UPLARR_SFTP_MAX_REQUESTS` (default `128`)

Concurrent outstanding requests per file, raised from the library default of 64
to improve throughput on high-bandwidth or high-latency links.

**Compatibility notes for both settings:**
- **OpenSSH sshd**: handles 128 concurrent requests and packets well above 32 KiB.
- **ProFTPD mod_sftp**: some configurations with strict per-connection request limits may reject transfers. If you see disconnects, drop requests to `64` and keep the packet size at `32768`.
- **FileZilla Server**: generally works, but restrictive configurations may need lower values.

If transfers begin failing after a change, revert to `UPLARR_SFTP_MAX_PACKET=32768`
first — an oversized packet is the more likely culprit, and it presents as an
unhelpful mid-transfer disconnect rather than a clear error.

---

## 🧪 Testing & Quality

Uplarr maintains **97.9% code coverage**, ensuring every critical path is verified.

```bash
# Run full suite
go test -v ./...

# View coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

---

## 🤝 Contributing

We welcome contributions! Please follow our streamlined workflow:

1. Fork the Project.
2. Create a Feature Branch (`git checkout -b feature/AmazingFeature`).
3. Commit Changes (`git commit -m 'Add some AmazingFeature'`).
4. Push to the Branch (`git push origin feature/AmazingFeature`).
5. Open a Pull Request to the `v2.2` branch.

---

## 📄 License

Distributed under the **MIT License**. See `LICENSE` for more information.

---
<p align="center">
  <i>Built with ❤️ using Go and Vanilla JS.</i>
</p>
