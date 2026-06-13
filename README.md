# USB Wiper

Securely wipe USB storage devices from your browser.

![USB Wiper Screenshot](docs/screenshot.png)

## Features

- **Multi-Scheme Wipe** — Choose from Zero Fill, Random Fill, DoD 5220.22-M 3-pass, NIST 800-88 Clear, and ATA Secure Erase.
- **Job Queue** — Queue multiple devices with configurable concurrency (default 2 simultaneous wipes). ULID-based job tracking.
- **Multi-Pass Progress** — Per-pass progress bars for multi-pass schemes (e.g. Pass 2/3).
- **Wipe Presets** — Named, reusable wipe configurations (scheme, auto-format, verify size, label template). Built-in presets: Quick Zero, Standard Sanitize, DoD 3-Pass, Paranoid.
- **Certificates of Erasure** — Tamper-evident PDF and JSON certificates signed with Ed25519. Verify certificates offline.
- **Audit Log** — Append-only JSON audit trail of all user actions and security events.
- **Physical Device Identity** — Tracks the disk behind USB bridges using SMART WWN/model/serial/capacity instead of the USB adapter ID.
- **Wipe History** — Persistent JSON-backed log with verification results and trusted physical device identity.
- **SMART Health History** — NVMe and ATA health info via smartctl with multi-bridge auto-detection, plus local history by physical device.
- **Auto Wipe** — Optional, disabled-by-default mode that queues the default wipe scheme when a newly attached trusted disk serial appears. Already-connected drives are marked seen when enabling.
- **Batch Wipe Configurator** — Multi-select one or many devices and drive them through a single focused configurator (targets, scheme/preset, options) gated by a deliberate hold-to-confirm, with a reduced-motion / keyboard fallback.
- **Real-time UI** — Modern, grouped sidebar layout with a live wipe dashboard (animated progress gauge), devices view with inspection drawer, queue panel, history viewer, activity log, Auto Wipe, presets editor, and settings page. Dark/light themes with depth and tasteful motion (all gated behind `prefers-reduced-motion`).
- **Server-Sent Events** — Push-driven progress updates, device plug/unplug detection.
- **Auto-Format** — Optional FAT32 formatting after wipe.
- **Webhook Notifications** — POST webhook on job completion.
- **Prometheus Metrics** — `/metrics` endpoint with counters and gauges.
- **Docker** — Single container, Debian stable-slim base.

## Quick Start

### Option 1: Pull from Docker Hub (Recommended)

```bash
docker run --rm -it --privileged \
  -v /dev:/dev \
  -v /sys:/sys:ro \
  -v /path/to/data:/data \
  -p 8181:8181 \
  michealchoudhary/usb-wiper:latest
```

Open [http://localhost:8181](http://localhost:8181) in your browser.

### Option 2: Docker compose

```yaml
services:
  usb-wiper:
    image: michealchoudhary/usb-wiper:latest
    container_name: usb-wiper
    privileged: true
    volumes:
      - /dev:/dev
      - /sys:/sys:ro
      - ./data:/data
    ports:
      - "8181:8181"
    environment:
      - UNSAFE_ALLOW_ALL_USB=1
      - ALLOW_HARDWARE_SECURE_ERASE=1
      - LOG_LEVEL=info
    restart: unless-stopped
```

```bash
docker compose up -d
```

### Option 3: Clone and build

```bash
git clone https://github.com/michealch/usb-wiper.git
cd usb-wiper
make prod
```

### Option 4: Run without Docker (Linux only)

```bash
make build
sudo UNSAFE_ALLOW_ALL_USB=1 ./bin/usb-wiper
```

## Environment Variables

| Variable                       | Default          | Description |
|-------------------------------|------------------|-------------|
| `PORT`                        | `8181`           | HTTP port |
| `LOG_LEVEL`                   | `info`           | Log level: `debug`, `info`, `warn`, `error` |
| `DATA_DIR`                    | `/data`          | Data directory for history, SMART health history, auto-wipe state, presets, settings, certificates, audit log |
| `METRICS_BIND`                | `127.0.0.1:9090` | Prometheus metrics listen address (set to `off` to disable) |
| `UNSAFE_ALLOW_ALL_USB`        | (unset)          | Skip removable-device check for USB SSDs/enclosures |
| `ALLOW_HARDWARE_SECURE_ERASE` | (unset)          | Enable ATA Secure Erase / NVMe Format scheme |
| `UNSAFE_ALLOW_PRIVATE_WEBHOOKS` | (unset)        | Allow webhook URLs pointing to private/internal IPs |

## API Reference

### Device & Health

| Method | Path                  | Description |
|--------|-----------------------|-------------|
| GET    | `/api/devices`        | List USB devices with status |
| GET    | `/api/health?device=X`| SMART health info |
| GET    | `/api/health-history?deviceId=X` | SMART health snapshots for a trusted physical device ID |

### Wipe Operations

| Method | Path                  | Description |
|--------|-----------------------|-------------|
| POST   | `/api/wipe`           | Start wipe (supports multi-device, scheme, preset) |
| POST   | `/api/test-wipe`      | Read-only verification |
| POST   | `/api/cancel`         | Cancel by device or all |

### Jobs

| Method | Path                  | Description |
|--------|-----------------------|-------------|
| GET    | `/api/jobs`           | List all jobs |
| GET    | `/api/jobs/{id}`      | Get single job |
| POST   | `/api/jobs/{id}/cancel`| Cancel specific job |

### Schemes & Presets

| Method | Path                  | Description |
|--------|-----------------------|-------------|
| GET    | `/api/schemes`        | List wipe schemes |
| GET    | `/api/presets`        | List presets |
| POST   | `/api/presets`        | Create preset |
| PUT    | `/api/presets/{id}`   | Update preset |
| DELETE | `/api/presets/{id}`   | Delete preset |

### Settings

| Method | Path                  | Description |
|--------|-----------------------|-------------|
| GET    | `/api/settings`       | Get settings |
| PUT    | `/api/settings`       | Update settings |
| GET    | `/api/config`         | Get config (backward compat) |
| POST   | `/api/config`         | Update config (backward compat) |

### Auto Wipe

| Method | Path                  | Description |
|--------|-----------------------|-------------|
| GET    | `/api/autowipe`       | Get Auto Wipe status, default action, and seen-device ledger |
| PUT    | `/api/autowipe`       | Enable or disable Auto Wipe (`{"enabled": true}`) |
| DELETE | `/api/autowipe/seen`  | Clear the seen-device ledger |

### Certificates

| Method | Path                  | Description |
|--------|-----------------------|-------------|
| GET    | `/api/cert/pubkey`    | Get Ed25519 public key |
| GET    | `/api/cert/{jobId}/json`| Download JSON certificate |
| GET    | `/api/cert/{jobId}/pdf` | Download PDF certificate |
| POST   | `/api/cert/verify`    | Verify certificate signature |

### Audit & Observability

| Method | Path                  | Description |
|--------|-----------------------|-------------|
| GET    | `/api/audit`          | Read audit log |
| GET    | `/metrics`            | Prometheus metrics |
| GET    | `/healthz`            | Liveness probe |
| GET    | `/api/events`         | SSE stream |

### History

| Method | Path                   | Description |
|--------|------------------------|-------------|
| GET    | `/api/history`         | All wipe history |
| GET    | `/api/history?device=X`| Wipe history by current device path |
| GET    | `/api/history?deviceId=X` | Wipe history by trusted physical device ID |

### POST /api/wipe

```json
{
  "devices": ["/dev/sdb", "/dev/sdc"],
  "schemeId": "dod-3pass",
  "presetId": "optional-preset-id",
  "autoFormat": true,
  "verifySizeGB": 4,
  "label": "RMA-2026-05"
}
```

## Wipe Schemes

| ID            | Name                       | Passes | Description |
|---------------|----------------------------|--------|-------------|
| `zero`        | Zero Fill                  | 1      | Single pass of zeros — fast, effective |
| `random`      | Random Fill                | 1      | Single pass of crypto-random data |
| `dod-3pass`   | DoD 5220.22-M 3-Pass       | 3      | Zeros, 0xFF, random — historically classified |
| `nist-clear`  | NIST 800-88 Clear          | 1      | Zero pass with NIST-compliant metadata |
| `secure-erase`| ATA Secure Erase / NVMe Format | 1  | Firmware-level sanitize (requires `ALLOW_HARDWARE_SECURE_ERASE=1`) |

## Safety Checks (MUST NOT BE MODIFIED)

All 9 checks run before every destructive operation:

1. Path must match `/dev/sd[a-z]`
2. Device must exist
3. Must be a block device
4. Must not be NVMe
5. Must not be the root device
6. Must be marked as removable (skipped when `UNSAFE_ALLOW_ALL_USB=1`)
7. Must be on a USB bus
8. Must not be mounted at system paths (`/`, `/boot`, `/home`, `/var`, `/usr`, `/etc`)
9. Size must be ≤ 2 TB

## Device Identity and Auto Wipe

USB-to-SATA/NVMe adapters can expose the same USB bridge identity after a disk
swap. USB Wiper therefore stores history against a physical disk identity when
possible:

1. SMART WWN (`high` confidence)
2. SMART model + serial + capacity (`high` confidence)
3. sysfs model + serial + capacity (`medium` confidence)
4. attachment/diskseq fallback (`low` confidence)

Only high/medium confidence identities are used for cross-attachment history.
Low-confidence fallback IDs are treated as attachment-scoped and are not used
for Auto Wipe eligibility.

Auto Wipe is disabled by default. When enabled from the Auto Wipe page or API,
currently attached trusted-serial devices are marked as seen first, so enabling
the feature does not wipe drives that are already connected. Later, when a new
trusted serial appears, the server queues a wipe with the current default scheme,
`autoFormat`, and `verifySizeGB` settings. The normal safety checks still run
before queueing and again in the queue worker.

## Development

```bash
make build          # Build local binary
make run            # Run locally (requires sudo)
make test           # Run tests with race detector
make test-verbose   # Verbose tests with race detector
make test-norace    # Run tests without race detector
make fmt            # Format code
make vet            # Run go vet
make lint           # Linter (requires golangci-lint)
make tidy           # Go mod tidy
make dev            # Dev compose with hot reload (air)
make dev-detached   # Dev compose in background
make docker-build   # Build production Docker image
make prod           # Production compose (detached)
make stop           # Stop all compose services
make logs           # Tail dev container logs
make shell          # Shell into dev container
make clean          # Remove build artifacts
```

## Architecture

```
cmd/usb-wiper/         → Entry point
internal/
  audit/               → Append-only audit log (JSON lines)
  cert/                → Ed25519-signed certificates (PDF + JSON)
  config/              → In-memory + persisted settings
  device/              → USB detection, safety checks, SMART
  format/              → FAT32 formatting
  metrics/             → Prometheus metrics registry
  notify/              → Webhook notifications
  persistence/         → JSON wipe history, SMART history, auto-wipe state
  presets/             → Named reusable wipe presets
  queue/               → FIFO job queue with concurrency control
  scheduler/           → Legacy scheduler package; not exposed in live UI/API
  server/              → HTTP server, SSE hub, all handlers
  server/static/       → Embedded web UI (ES modules, dark/light themes)
  ulid/                → ULID generator (stdlib-only)
  wipe/                → Multi-scheme wipe engine + verification
```

## Tech Stack

- **Go 1.26+** (stdlib only)
- **Vanilla HTML5 + ES modules + CSS custom properties** (no frameworks, no build step)
- **Server-Sent Events** for real-time progress
- **Docker** with Debian stable-slim
- **Ed25519** signing for certificates
- **MIT License**
