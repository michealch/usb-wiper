# AGENTS.md — USB Wiper Specification for AI Coding Agents

## Project Summary

USB Wiper is a secure USB device wiping tool. It writes zeros across the entire
USB device, displays SMART/health information, provides a minimal web UI with
real-time progress via Server-Sent Events, and optionally formats as FAT32 after
wiping. Runs as a single Docker container.

## Tech Stack (Locked)

- **Language:** Go 1.22+
- **Dependencies:** Standard library only — NO external Go modules
- **HTTP:** `net/http`
- **Templates:** `html/template`
- **Frontend:** Plain HTML5 + vanilla JS + CSS (no frameworks, no build steps)
- **Realtime:** Server-Sent Events (SSE)
- **Tools (shelled out):** `smartctl`, `mkfs.vfat`, `parted`, `lsblk`
- **Container base:** `alpine:3.19`
- **Build base:** `golang:1.22-alpine`
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
| POST   | /api/wipe             | Start wipe (autoFormat bool) |
| POST   | /api/cancel           | Cancel active wipe           |
| GET    | /api/job              | Current job status           |
| GET    | /api/config           | Get config                   |
| POST   | /api/config           | Update config                |
| GET    | /api/events           | SSE stream                   |
| GET    | /healthz              | Liveness probe               |

All errors: `{"error": "message"}` with HTTP 4xx/5xx.

## Development

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
