# Changelog

All notable changes to Velkron Pulse are documented in this file.

## [1.1.0] - 2026-05-30

### Added
- Optional JSON config file at `{db-path}/config.json` (CLI flags override file values)
- Bearer-authenticated `/api/info` and `/api/metrics/prometheus` endpoints
- Endpoint uptime tracking (24-hour probe history) with UI column
- Metrics history chart on the overview dashboard (1-hour CPU/memory sparklines)
- Settings panel: version, bind address, port, masked token, copy-token button
- Authenticated export downloads from the UI (JSON/CSV)
- Structured logging via `log/slog`
- GitHub Actions CI workflow (test + build)
- Reverse-proxy deployment documentation

### Changed
- Version badges now injected at runtime (single source of truth from server)
- WebSocket auth uses HttpOnly cookie instead of query-string token
- API token masked in startup logs
- Response time formatting corrected for nanosecond values

### Security
- All improvements from v1.0.3 security hardening retained (loopback bind default, bearer auth, SSRF guards, rate limits)

## [1.0.3] - 2026-05-30

### Added
- `--version` flag
- Live system info in metrics (hostname, OS, CPU cores)
- Inline delete button for custom endpoints

### Security
- Default bind address `127.0.0.1`
- Bearer token authentication for API and WebSocket
- SSRF validation for custom endpoints
- Rate limiting, request body limits, security headers
- Settings key allowlist

## [1.0.2] - 2026-05-30

- Dashboard overlays for hostname, platform, and core count
- Custom endpoint delete button in UI

## [1.0.1] - 2026-05-30

- Stability, polish, and observability improvements

## [1.0.0] - 2026-05-30

- Initial release
