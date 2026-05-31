# Velkron Pulse

**Lightweight, single-binary infrastructure monitoring agent with an embedded real-time web dashboard.**

Download one file, run it, and immediately get a beautiful real-time view of your system and services — no cloud, no dependencies, no setup.

![Velkron Pulse Dashboard](https://placeholder.velkron.com/pulse-screenshot.png)

---

## Features

- **System Metrics** — CPU usage (gauge), memory (bar), disk usage per mount point, network I/O per interface, system uptime
- **Service Discovery** — Auto-detects common services (nginx, PostgreSQL, Redis, MySQL, Docker, MongoDB, Elasticsearch, Prometheus, Grafana) by checking default ports
- **Custom Endpoints** — Add your own HTTP or TCP endpoints to monitor from the UI
- **Real-Time Dashboard** — Dark-mode SPA with live WebSocket updates every 2 seconds
- **Alerting** — In-browser notifications when disk usage exceeds threshold or a service goes down
- **Export** — Download current snapshot as JSON or CSV
- **Persistence** — Custom endpoints, settings, and 24-hour metrics history stored in local SQLite
- **Cross-Platform** — Single static binary for Linux (amd64/arm64), macOS (amd64/arm64), Windows (amd64)

---

## Quick Start

### Prerequisites

- **Go 1.21+** — [Download Go](https://go.dev/dl/)

### Build & Run

**Windows (PowerShell):**
```powershell
cd C:\VelkronPulse
$env:CGO_ENABLED=0
go build -ldflags="-s -w" -o velkron-pulse.exe .
.\velkron-pulse.exe
```

**Windows (cmd.exe):**
```cmd
cd C:\VelkronPulse
set CGO_ENABLED=0 && go build -ldflags="-s -w" -o velkron-pulse.exe .
velkron-pulse.exe
```

**Linux / macOS:**
```bash
cd velkron-pulse
CGO_ENABLED=0 go build -ldflags="-s -w" -o velkron-pulse .
./velkron-pulse
```

Your browser will automatically open to `http://localhost:2024`.

### Command-Line Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | `2024` | HTTP server port |
| `--bind` | `127.0.0.1` | Network address to bind (`0.0.0.0` for all interfaces) |
| `--db-path` | `~/.velkron-pulse/` | Database directory |
| `--refresh` | `2` | Metrics collection interval (seconds) |
| `--no-browser` | `false` | Disable auto-opening browser |
| `--token` | *(auto)* | API bearer token (also `VELKRON_PULSE_TOKEN`) |
| `--version` | — | Print version and exit |

Example:
```bash
./velkron-pulse --port 8080 --refresh 5 --no-browser --token "$(openssl rand -hex 32)"
```

---

## Security

Velkron Pulse is a **local monitoring agent**. By default it binds to **loopback only** (`127.0.0.1`) and requires a **bearer token** for all API and WebSocket access.

- On startup, a masked API token is logged and injected into the dashboard automatically.
- Use `--bind 0.0.0.0` only on trusted networks with a strong `--token` and a host firewall.
- Custom endpoints block cloud metadata addresses and disallow HTTP redirects.
- Settings writes are restricted to known keys (`disk_threshold`, `cpu_threshold`).
- For remote access, put a TLS-terminating reverse proxy with authentication in front of Pulse.

API clients must send `Authorization: Bearer <token>` on every request except `GET /api/health`.

### Optional config file

Place `config.json` in your database directory (see `config.example.json`):

```json
{
  "port": 2024,
  "bind": "127.0.0.1",
  "refresh": 2,
  "no_browser": false
}
```

CLI flags override values from the config file.

### Reverse proxy (TLS)

For LAN access, terminate TLS in front of Pulse and keep Pulse on loopback:

```text
Client ──HTTPS──▶ Caddy/nginx ──HTTP──▶ 127.0.0.1:2024
```

Do not expose port 2024 directly without TLS and a strong token.

### Prometheus scrape

Authenticated endpoint: `GET /api/metrics/prometheus` with `Authorization: Bearer <token>`.

---

## Cross-Compilation

Build for all target platforms at once:

**PowerShell (cross-compile):**
```powershell
$env:CGO_ENABLED=0
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -ldflags="-s -w" -o build/velkron-pulse-linux-amd64 .
$env:GOOS="linux"; $env:GOARCH="arm64"; go build -ldflags="-s -w" -o build/velkron-pulse-linux-arm64 .
$env:GOOS="darwin"; $env:GOARCH="amd64"; go build -ldflags="-s -w" -o build/velkron-pulse-darwin-amd64 .
$env:GOOS="darwin"; $env:GOARCH="arm64"; go build -ldflags="-s -w" -o build/velkron-pulse-darwin-arm64 .
$env:GOOS="windows"; $env:GOARCH="amd64"; go build -ldflags="-s -w" -o build/velkron-pulse-windows-amd64.exe .
```

**Linux / macOS (cross-compile):**
```bash
# Linux amd64
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/velkron-pulse-linux-amd64 .

# Linux arm64
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/velkron-pulse-linux-arm64 .

# macOS amd64 (Intel)
GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/velkron-pulse-darwin-amd64 .

# macOS arm64 (Apple Silicon)
GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/velkron-pulse-darwin-arm64 .

# Windows amd64
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o build/velkron-pulse-windows-amd64.exe .
```

Or use the included build script (Linux/macOS):
```bash
chmod +x build/build-all.sh
./build/build-all.sh
```

---

## API Reference

| Method | Endpoint | Auth | Description |
|--------|----------|------|-------------|
| `GET` | `/api/health` | No | Health check |
| `GET` | `/api/status` | Bearer | Current metrics + services snapshot |
| `GET` | `/api/endpoints` | Bearer | List custom endpoints |
| `POST` | `/api/endpoints` | Bearer | Add custom endpoint `{name, url, type}` |
| `DELETE` | `/api/endpoints/{id}` | Bearer | Remove custom endpoint |
| `GET` | `/api/settings` | Bearer | Get alert thresholds |
| `PUT` | `/api/settings` | Bearer | Update setting `{key, value}` |
| `GET` | `/api/metrics/history?from=&to=` | Bearer | Historical metrics (RFC3339 timestamps) |
| `GET` | `/api/export/json` | Bearer | Download snapshot as JSON |
| `GET` | `/api/export/csv` | Bearer | Download snapshot as CSV |
| `GET` | `/api/info` | Bearer | Version, bind address, and port |
| `GET` | `/api/metrics/prometheus` | Bearer | Prometheus text metrics |
| `GET` | `/ws` | Cookie/Bearer | WebSocket for real-time updates |

---

## Project Structure

```
velkron-pulse/
├── main.go                  # Entry point
├── embed.go                 # //go:embed directive
├── go.mod / go.sum          # Go module
├── internal/
│   ├── config/config.go     # CLI flag parsing
│   ├── metrics/collector.go # System metrics collection
│   ├── services/scanner.go  # Service discovery & health checks
│   ├── store/db.go          # SQLite persistence
│   └── web/
│       ├── server.go        # HTTP server & API handlers
│       └── ws.go            # WebSocket hub
├── web/public/
│   ├── index.html           # SPA dashboard
│   ├── global.css           # Dark theme styles
│   └── build.js             # Frontend application
├── build/build-all.sh       # Cross-compilation script
└── README.md
```

---

## Technology Stack

- **Language:** Go (static binary, CGO-free)
- **Frontend:** Vanilla HTML/CSS/JS (embedded via `//go:embed`)
- **Database:** SQLite via `modernc.org/sqlite` (pure Go)
- **Real-time:** WebSocket via `gorilla/websocket`
- **System Metrics:** `shirou/gopsutil/v3`
- **HTTP Routing:** `gorilla/mux`

---

## License

**Freeware** — This software is free to use and distribute in binary form. Source code is not publicly licensed for redistribution.

---

*Powered by [Velkron Technologies](https://velkron.com)*
