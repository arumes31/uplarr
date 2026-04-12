# 🚀 Uplarr

[![Go Version](https://img.shields.io/github/go-mod/go-version/arumes31/uplarr?style=flat-square)](https://go.dev/)
[![CI Status](https://img.shields.io/github/actions/workflow/status/arumes31/uplarr/ci.yml?branch=main&style=flat-square)](https://github.com/arumes31/uplarr/actions)
[![Docker Image](https://img.shields.io/badge/docker-ghcr.io-blue?style=flat-square&logo=docker)](https://github.com/arumes31/uplarr/pkgs/container/uplarr)
[![License](https://img.shields.io/badge/license-MIT-green?style=flat-square)](LICENSE)

**Uplarr** is a high-performance, zero-bloat Go application designed to bridge the gap between local storage and remote SFTP servers. With a sleek modern Web GUI, real-time progress logging via SSE, and robust verification logic, Uplarr ensures your data moves safely and efficiently.

---

## ✨ Key Features

- 🛠 **Dynamic Configuration**: No more hardcoded env vars. Configure and test your SFTP connections directly in the browser.
- 📡 **Real-time SSE Logs**: Watch your uploads happen live with integrated Server-Sent Events logging.
- ✅ **Verification Suite**: Automatic remote file integrity checks via size comparison after every upload.
- 🧹 **Smart Cleanup**: Optional automatic deletion of local files only after successful remote verification.
- 🐳 **Multi-Arch Docker**: Official support for `amd64` and `arm64` via GHCR.
- ⚡ **Go 1.26 Powered**: Leveraging the latest Go performance and security enhancements.

---

## 📸 Web Interface

> *Clean, Responsive, and Fast.*

- **Dashboard**: View local files, their sizes, and status.
- **Config**: Reactive form with host, port, user, and credential management.
- **Live Logs**: Dedicated terminal-style window for real-time process feedback.

---

## 🛠 Quick Start

### Using Docker (Recommended)

```bash
docker run -d \
  -p 8080:8080 \
  -v /your/local/data:/root/test_data \
  --name uplarr \
  ghcr.io/arumes31/uplarr:latest
```

### Local Development

1. **Prerequisites**: Go 1.26+ installed.
2. **Install**:
   ```bash
   go mod download
   ```
3. **Run**:
   ```bash
   go run .
   ```
4. **Access**: Open [http://localhost:8080](http://localhost:8080) in your browser.

---

## ⚙️ Configuration (Environment Variables)

| Variable | Description | Default |
| :--- | :--- | :--- |
| `LOCAL_DIR` | Directory to monitor for files | `./test_data` |
| `WEB_PORT` | Port for the Web GUI | `8080` |

*SFTP credentials, host settings, and host key verification are managed via the Web GUI.*

---

## 🧪 Testing

We maintain a high-quality codebase with extensive test coverage.

**Run tests locally:**
```bash
go test -v ./...
```

**Run with coverage report:**
```bash
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

---

## 🚢 Deployment & CI/CD

Uplarr uses GitHub Actions for continuous integration and deployment:
- **CI**: Automated testing on every push to `main` and `v2_test`.
- **Release**: Multi-platform Docker builds (`amd64`, `arm64`) pushed to GHCR on main branch updates.

---

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the Project
2. Create your Feature Branch (`git checkout -b feature/AmazingFeature`)
3. Commit your Changes (`git commit -m 'Add some AmazingFeature'`)
4. Push to the Branch (`git push origin feature/AmazingFeature`)
5. Open a Pull Request

---

## 📄 License

Distributed under the MIT License. See `LICENSE` for more information.

---
*Built with ❤️ using Go and Vanilla JS.*
