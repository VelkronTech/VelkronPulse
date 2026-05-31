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
| `--db-path` | `~/.velkron-pulse/` | Database directory |
| `--refresh` | `2` | Metrics collection interval (seconds) |
| `--no-browser` | `false` | Disable auto-opening browser |

Example:
```bash
./velkron-pulse --port 8080 --refresh 5 --no-browser
```

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

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/status` | Current metrics + services snapshot |
| `GET` | `/api/endpoints` | List custom endpoints |
| `POST` | `/api/endpoints` | Add custom endpoint `{name, url, type}` |
| `DELETE` | `/api/endpoints/{id}` | Remove custom endpoint |
| `GET` | `/api/settings` | Get all settings |
| `PUT` | `/api/settings` | Update setting `{key, value}` |
| `GET` | `/api/metrics/history?from=&to=` | Historical metrics (RFC3339 timestamps) |
| `GET` | `/api/export/json` | Download snapshot as JSON |
| `GET` | `/api/export/csv` | Download snapshot as CSV |
| `GET` | `/ws` | WebSocket endpoint for real-time updates |

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
