# AGENTS.md — USB Wiper Specification for AI Coding Agents

## Project Summary

USB Wiper is a secure USB device wiping tool. It writes zeros across the entire
USB device, displays SMART/health information, provides a minimal web UI with
real-time progress via Server-Sent Events, and optionally formats as FAT32 after
wiping. Runs as a single Docker container.

## Tech Stack (Locked)

- **Language:** Go 1.26+
- **Dependencies:** Standard library only — NO external Go modules
- **HTTP:** `net/http`
- **Templates:** `html/template`
- **Frontend:** Plain HTML5 + vanilla JS + CSS (no frameworks, no build steps)
- **Realtime:** Server-Sent Events (SSE) — fully push-driven, no polling
- **Device detection:** Background watcher scans /sys/block every 3s, pushes SSE refresh on change
- **Tools (shelled out):** `smartctl`, `mkfs.vfat`, `parted`, `lsblk`
- **Container base:** `debian:stable-slim`
- **Build base:** `golang:1.26-alpine`
- **Persistence:** JSON file (`$DATA_DIR/history.json`), atomic writes
- **License:** MIT

## Non-Negotiables

1. **Safety rules MUST NOT be weakened.** The checks in `internal/device/safety.go`
   are exhaustive and ordered. Never remove or simplify any check.
2. **Always call `IsSafeToWipe`** before every destructive operation (wipe, format).
3. **Never accept user-provided device paths** without server-side re-validation.
4. **Never log device contents.**
5. **No external Go dependencies.** Stdlib only.

## API Contract

| Method | Path                  | Notes                        |
|--------|-----------------------|------------------------------|
| GET    | /api/devices          | List USB devices             |
| GET    | /api/health?device=X  | SMART health                 |
| GET    | /api/history?device=X | Wipe history (all or by device) |
| POST   | /api/wipe             | Start wipe (device, autoFormat, verifySizeGB) |
| POST   | /api/cancel           | Cancel active wipe           |
| GET    | /api/job              | Current job status           |
| GET    | /api/config           | Get config                   |
| POST   | /api/config           | Update config                |
| GET    | /api/events           | SSE stream                   |
| GET    | /healthz              | Liveness probe               |

All errors: `{"error": "message"}` with HTTP 4xx/5xx.

## SSE Protocol

Events are JSON objects with an `eventType` field:

| eventType | When | Frontend behavior |
|-----------|------|-------------------|
| (unset/"progress") | Wipe progress, verification progress | Update per-row progress bar + status |
| `"refresh"` | Wipe started/completed/cancelled, device plugged/unplugged | Full reload of device list and history |

The server's `watchDevices` goroutine scans for USB device changes every 3s.
When a change is detected (plug/unplug), a refresh event is broadcast.

## Key Files

| Path | Purpose |
|------|---------|
| `cmd/usb-wiper/main.go` | Entry point |
| `internal/server/server.go` | HTTP server, route registration |
| `internal/server/handlers.go` | API handlers, job manager, wipe orchestration |
| `internal/server/sse.go` | Server-Sent Events hub |
| `internal/server/middleware.go` | Logging, recovery, CORS |
| `internal/wipe/wipe.go` | Zero-write wipe + random-chunk verification |
| `internal/wipe/progress.go` | Progress event type + speed calculation |
| `internal/device/detect.go` | USB device detection via sysfs |
| `internal/device/safety.go` | Safety checks (DON'T MODIFY) |
| `internal/device/smart.go` | SMART health via smartctl |
| `internal/format/format.go` | FAT32 formatting |
| `internal/config/config.go` | In-memory config |
| `internal/persistence/persistence.go` | JSON-backed wipe history store |
| `internal/server/static/` | Embedded frontend (HTML, JS, CSS) |

## Development

### Persistence

Wipe history is stored as JSON at `$DATA_DIR/history.json` (default `/data/history.json`).
Records persist across container restarts. The file uses atomic write (temp + rename).

### Verification

After a successful zero-write, the server reads random 1 MiB chunks scattered
across the device. The total verification size is configurable via
`verifySizeGB` (config, default 1 GiB). Set to 0 to disable.

### Commands

```bash
make dev          # Start dev compose with hot reload
make test         # Run tests with race detector
make build        # Build local binary
make prod         # Start prod compose
```

## Safety Check Order

1. Path regex: `^/dev/sd[a-z]$`
2. `os.Stat` exists
3. Mode is `ModeDevice`
4. Not containing "nvme"
5. Not the root device
6. `removable == 1`
7. USB bus (`/usb` in device symlink)
8. Not mounted at system paths (`/`, `/boot`, `/home`, `/var`, `/usr`, `/etc`)
9. Size ≤ 2 TB
