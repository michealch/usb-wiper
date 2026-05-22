# USB Wiper

[![Docker Pulls](https://img.shields.io/docker/pulls/michealchoudhary/usb-wiper)](https://hub.docker.com/r/michealchoudhary/usb-wiper)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**Securely wipe USB storage devices from your browser.** Single Docker container, zero external Go dependencies, complete audit trail with Ed25519-signed certificates of erasure.

## Quick Start

```bash
docker run --rm -it --privileged \
  -v /dev:/dev \
  -v /sys:/sys:ro \
  -v /path/to/data:/data \
  -p 8181:8181 \
  michealchoudhary/usb-wiper:latest
```

Open [http://localhost:8181](http://localhost:8181).

## Why USB Wiper?

- **Trust but verify.** Every wipe is verified via random-chunk sampling. Certificates of erasure are signed with Ed25519 — tamper with any field and verification fails.
- **Real-time UI.** Progress bars, pass indicators (e.g. "Pass 2/3"), device plug/unplug detection — all pushed via Server-Sent Events. No page refresh needed.
- **Multiple wipe schemes.** Zero Fill, Random Fill, DoD 5220.22-M 3-pass, NIST 800-88 Clear, and ATA Secure Erase — pick the right level for your use case.
- **Queue awareness.** Queue up multiple devices, set concurrency limits, attach labels, choose presets. Jobs run FIFO, cancel any job individually.
- **Audit trail.** Append-only JSON audit log of every action. Compliant with chain-of-custody requirements.
- **Docker-native.** Runs as a single privileged container. No sidecars, no databases, no orchestration required.

## Features

| Category | Capabilities |
|----------|-------------|
| **Wipe Schemes** | Zero Fill, Random Fill, DoD 5220.22-M 3-pass, NIST 800-88 Clear, ATA Secure Erase |
| **Verification** | Random 1 MiB chunk sampling (configurable 0–16 GiB), non-destructive "test wipe" mode |
| **Certificates** | Tamper-evident PDF + JSON, Ed25519-signed, offline-verifiable |
| **Job Queue** | FIFO with configurable concurrency (1–4 simultaneous), ULID-based tracking, per-job cancel |
| **Presets** | Named reusable configurations (4 built-in + custom), label templates with `{date}`/`{datetime}` vars |
| **SMART Health** | ATA + NVMe health via smartctl with multi-bridge auto-detection |
| **UI** | Sidebar layout, device drawer with SMART/Wipe/History tabs, dark/light themes |
| **Real-time** | SSE push-driven — progress, device plug/unplug, job state changes, Last-Event-ID replay |
| **Safety** | 9 non-negotiable safety checks before every destructive operation, re-validated at execution time |
| **Audit** | Append-only JSON-lines audit log with correlation IDs, 50 MB auto-rotation |
| **Metrics** | Prometheus `/metrics` endpoint with counters and gauges |
| **HTTP** | Request correlation ULIDs via `X-Request-ID` header, structured logging |
| **Notifications** | Webhook on job completion (SSRF-protected, private-IP blocked by default) |
| **History** | Persistent JSON-backed wipe log with atomic writes |

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8181` | HTTP port |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `DATA_DIR` | `/data` | Persistent data directory |
| `METRICS_BIND` | `127.0.0.1:9090` | Prometheus metrics listen address (set to `off` to disable) |
| `UNSAFE_ALLOW_ALL_USB` | (unset) | Skip removable-device check for USB SSD enclosures |
| `ALLOW_HARDWARE_SECURE_ERASE` | (unset) | Enable ATA Secure Erase / NVMe Format scheme |
| `UNSAFE_ALLOW_PRIVATE_WEBHOOKS` | (unset) | Allow webhook URLs pointing to private IPs |

### Volumes

| Path | Purpose |
|------|---------|
| `/dev` | Block devices (required, read-write) |
| `/sys` | Sysfs for device detection (required, read-only) |
| `/data` | History, settings, presets, certificates, audit log (persistent) |

### Docker Compose

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
      - LOG_LEVEL=info
    restart: unless-stopped
```

## Wipe Schemes

| ID | Name | Passes | When to Use |
|----|------|--------|-------------|
| `zero` | Zero Fill | 1 | Everyday decommissioning — fast, effective |
| `random` | Random Fill | 1 | Crypto-random overwrite |
| `dod-3pass` | DoD 5220.22-M | 3 | Historically used for classified data |
| `nist-clear` | NIST 800-88 Clear | 1 | Zero pass with NIST-compliant metadata |
| `secure-erase` | ATA Secure Erase | 1 | Firmware-level sanitize (requires env var) |

## Safety Guarantees

Every destructive operation passes **9 independent safety checks** in order:

1. Path regex: `^/dev/sd[a-z]$` — blocks partitions, NVMe, loop devices
2. Device must exist on the filesystem
3. Must be a block device (`ModeDevice`)
4. Must not be NVMe
5. Must not be the root device
6. Must be marked `removable == 1` (skipped when `UNSAFE_ALLOW_ALL_USB=1`)
7. Must be on a USB bus (`/usb` in device symlink)
8. Must not be mounted at `/`, `/boot`, `/home`, `/var`, `/usr`, `/etc`
9. Size must be ≤ 2 TB

These checks run at request time **and again** right before each destructive operation (wipe, verify, format) — the device cannot change between queue and execution.

## Certificate of Erasure

Every completed wipe can generate a tamper-evident certificate:

- **PDF + JSON** formats available for download
- **Ed25519 signature** over canonical JSON — altering any field (device, timestamps, scheme, result) invalidates the signature
- **Offline verification** — download the public key, verify with any Ed25519 library (`openssl`, `golang.org/x/crypto`, etc.)
- **Cryptographic hashes** — SHA-256 of pre/post wipe device samples for chain-of-custody evidence

## Requirements

- **Docker** with `privileged: true` — required for `/dev` access and hotplug device nodes
- **Linux host** — the sysfs-based device detection and ioctl operations are Linux-specific
- **Port 8181** accessible from your browser

## Security

- **No external Go dependencies** — stdlib only; supply chain attack surface is minimal
- **Webhook SSRF protection** — webhook URLs targeting private/loopback IPs are rejected unless explicitly opted in
- **CORS restricted to local origins** — parsed via `net.ParseIP`, not prefix matching
- **Certificate body capped at 1 MiB** — prevents memory exhaustion on verify endpoint
- **Audit log is append-only with per-write `f.Sync()`** — crash-safe event trail

## Development

```bash
git clone https://github.com/michealch/usb-wiper.git
cd usb-wiper
make dev     # hot-reload dev server
make test    # run tests
make build   # build binary
make prod    # production Docker deployment
```

Tech stack: Go 1.26+ (stdlib only), vanilla HTML5 + ES modules + CSS custom properties, Server-Sent Events, Debian stable-slim.

## License

MIT — see [LICENSE](LICENSE).
