# AGENTS.md — USB Wiper Specification for AI Coding Agents

## Project Summary

USB Wiper is a secure USB device wiping tool. It supports multiple wipe schemes (Zero Fill, Random Fill, DoD 5220.22-M 3-pass, NIST 800-88 Clear, ATA Secure Erase), verifies via random-chunk sampling, generates tamper-evident Ed25519-signed certificates of erasure (PDF + JSON), maintains an append-only audit log, optionally formats as FAT32 after wiping, and provides a real-time web UI with dashboard, devices drawer, queue, presets, history, settings, and dark/light themes. Runs as a single Docker container.

## Tech Stack (Locked)

- **Language:** Go 1.26+
- **Dependencies:** Standard library only — NO external Go modules
- **HTTP:** `net/http`
- **Templates:** Vanilla HTML5 + ES modules + CSS custom properties (no frameworks, no build steps)
- **Realtime:** Server-Sent Events (SSE) with named events — fully push-driven, no polling
- **Device detection:** Background watcher scans /sys/block every 3s, pushes SSE refresh on change
- **Certificates:** `crypto/ed25519`, pure-Go PDF 1.4 writer, `crypto/sha256` for hashing
- **Tools (shelled out):** `smartctl`, `mkfs.vfat`, `parted`, `lsblk`, `hdparm`, `nvme`
- **Container base:** `debian:stable-slim`
- **Build base:** `golang:1.26-alpine`
- **Persistence:** JSON files (`$DATA_DIR/history.json`, `settings.json`, `presets.json`, `schedules.json`, `audit.log`), atomic writes
- **License:** MIT

## Non-Negotiables

1. **Safety rules MUST NOT be weakened.** The checks in `internal/device/safety.go` are exhaustive and ordered. Never remove or simplify any check.
2. **Always call `IsSafeToWipe`** before every destructive operation (wipe, verify,
   format, secure erase) — including re-validation in the queue worker right
   before execution, not just at enqueue time.
3. **Never accept user-provided device paths** without server-side re-validation.
4. **Never log device contents.**
5. **No external Go dependencies.** Stdlib only.

## API Contract

| Method | Path                        | Notes                                    |
|--------|-----------------------------|------------------------------------------|
| GET    | /api/devices                | List USB devices                         |
| GET    | /api/health?device=X        | SMART health                             |
| GET    | /api/history?device=X       | Wipe history (all or by device)           |
| POST   | /api/wipe                   | Start wipe (devices, schemeId, presetId, autoFormat, verifySizeGB, label) |
| POST   | /api/test-wipe              | Read-only zero verification              |
| POST   | /api/cancel                 | Cancel active wipe (by device or all)     |
| GET    | /api/jobs                   | List all jobs (queue + history snapshot) |
| GET    | /api/jobs/{id}              | Get single job                           |
| POST   | /api/jobs/{id}/cancel       | Cancel specific job                      |
| GET    | /api/schemes                | List available wipe schemes              |
| GET    | /api/presets                | List presets                             |
| POST   | /api/presets                | Create preset                            |
| PUT    | /api/presets/{id}           | Update preset                            |
| DELETE | /api/presets/{id}           | Delete preset                            |
| GET    | /api/settings               | Get settings                             |
| PUT    | /api/settings               | Update settings                          |
| GET    | /api/config                 | Get config (backward compat)             |
| POST   | /api/config                 | Update config (backward compat)          |
| GET    | /api/cert/pubkey            | Get Ed25519 public key (PEM base64)      |
| GET    | /api/cert/{jobId}/json      | Download certificate JSON                |
| GET    | /api/cert/{jobId}/pdf       | Download certificate PDF                 |
| POST   | /api/cert/verify            | Verify certificate signature             |
| GET    | /api/audit                  | Read audit log (newest first, up to 200) |
| GET    | /metrics                    | Prometheus metrics                       |
| GET    | /api/events                 | SSE stream                               |
| GET    | /healthz                    | Liveness probe                           |

All errors: `{"error": "message"}` with HTTP 4xx/5xx.

## Wipe Schemes

| ID            | Passes | Description |
|---------------|--------|-------------|
| `zero`        | 1      | Zero fill |
| `random`      | 1      | Crypto-random fill |
| `dod-3pass`   | 3      | DoD 5220.22-M: zeros → 0xFF → random |
| `nist-clear`  | 1      | NIST 800-88 Clear (alias for zero with metadata) |
| `secure-erase`| 1      | ATA Secure Erase / NVMe Format (requires ALLOW_HARDWARE_SECURE_ERASE=1) |

## SSE Protocol

Events use named SSE event types for selective frontend subscription:

| Event Type | When | Frontend behavior |
|-----------|------|-------------------|
| `"refresh"` | Wipe started/completed/cancelled, device plugged/unplugged | Full reload of device list, jobs, and history |
| `"job"`     | Job state change (queued → running → complete) | Update queue/dashboard view |
| (unset/"progress") | Wipe progress, verification progress | Update per-row progress bar + pass indicator |

Progress events carry `currentPass` and `totalPasses` for multi-pass schemes.

## Key Files

| Path | Purpose |
|------|---------|
| `cmd/usb-wiper/main.go` | Entry point |
| `internal/server/server.go` | HTTP server, route registration, startup |
| `internal/server/handlers.go` | All API handlers (REST + SSE) |
| `internal/server/sse.go` | Server-Sent Events hub |
| `internal/server/middleware.go` | Logging, recovery, CORS |
| `internal/server/static/` | Embedded frontend (HTML, CSS, JS ES modules) |
| `internal/wipe/scheme.go` | Scheme interface + registry |
| `internal/wipe/scheme_zero.go` | Zero, Random, DoD, NIST implementations |
| `internal/wipe/scheme_secure_erase.go` | ATA/NVMe secure erase |
| `internal/wipe/wipe.go` | Legacy Wipe() + helpers (openDeviceWrite, BlkGetSize64) |
| `internal/wipe/progress.go` | ProgressEvent type + speed calculation |
| `internal/queue/queue.go` | FIFO job queue with concurrency semaphore |
| `internal/queue/queue_helpers.go` | Device I/O, formatting, history bridge |
| `internal/device/detect.go` | USB device detection via sysfs |
| `internal/device/safety.go` | Safety checks (DON'T MODIFY) |
| `internal/device/smart.go` | SMART health via smartctl multi-bridge |
| `internal/format/format.go` | FAT32 formatting |
| `internal/config/config.go` | In-memory + persisted settings |
| `internal/presets/presets.go` | Named reusable wipe presets |
| `internal/persistence/persistence.go` | JSON-backed wipe history store |
| `internal/cert/cert.go` | Ed25519-signed certificates |
| `internal/cert/pdf.go` | Pure-Go PDF 1.4 generator |
| `internal/audit/audit.go` | Append-only JSON audit log |
| `internal/scheduler/scheduler.go` | Cron/device-insert scheduler |
| `internal/notify/notify.go` | Webhook notifications |
| `internal/metrics/metrics.go` | Prometheus metrics registry |
| `internal/ulid/ulid.go` | Stdlib-only ULID generator |

## Safety Check Order

1. Path regex: `^/dev/sd[a-z]$`
2. `os.Stat` exists
3. Mode is `ModeDevice`
4. Not containing "nvme"
5. Not the root device
6. `removable == 1` (skipped when `UNSAFE_ALLOW_ALL_USB=1`)
7. USB bus (`/usb` in device symlink)
8. Not mounted at system paths (`/`, `/boot`, `/home`, `/var`, `/usr`, `/etc`)
9. Size ≤ 2 TB

## Persistence

All data stored under `$DATA_DIR/` (default `/data/`):

| File | Format | Purpose |
|------|--------|---------|
| `history.json` | JSON array | Wipe history (atomic write) |
| `settings.json` | JSON object | User settings (atomic write) |
| `presets.json` | JSON array | Wipe presets (atomic write) |
| `schedules.json` | JSON array | Wipe schedules (atomic write) |
| `audit.log` | JSON lines | Append-only audit trail |
| `keys/signing.ed25519` | Ed25519 seed | Certificate signing key (0600) |
| `keys/signing.ed25519.pub` | Ed25519 public key | Verification key (0644) |

## Development Commands

```bash
make dev          # Start dev compose with hot reload
make test         # Run tests
make build        # Build local binary
make prod         # Start prod compose
```

## Verification

After a successful wipe, the server reads random 1 MiB chunks scattered across the device. Total verification size is configurable via `verifySizeGB` (config, default 1 GiB). Set to 0 to disable.

## Certificate of Erasure

Each completed wipe generates a JSON certificate with Ed25519 signature over canonical JSON. A corresponding PDF is available for download from the job. Certificates include tool info, host info, device details, scheme details with timestamps, verification results, and optional pre/post wipe SHA-256 hashes.
