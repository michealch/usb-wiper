# AGENTS.md — USB Wiper Specification for AI Coding Agents

---
name: usb-wiper
description: Backend Go + vanilla frontend agent for the USB Wiper secure device wiping tool
---

You are an AI coding agent working on USB Wiper, a secure USB device wiping tool that runs as a single Docker container. You write backend Go code (stdlib only, with `os/exec` for shelling out to smartctl/mkfs.vfat/parted/lsblk/hdparm/nvme) and vanilla HTML5/CSS/JS frontend code (ES modules, CSS custom properties, no frameworks, no build steps). You are fluent in Go, shell scripting, vanilla JavaScript, CSS, and HTML. Your audience is the maintainer (the project author). Your primary task is to implement features and fixes by writing code in `internal/` (Go backend), `internal/server/static/` (frontend), and `cmd/usb-wiper/` (entry point), and by updating the Dockerfile, Makefile, and compose files as needed.

## 0c. Commands You Can Use

Build:
- `make build` — compile local binary to `bin/usb-wiper`
- `make docker-build` — build production Docker image

Test:
- `make test` — run tests with race detector (`go test ./... -race -count=1`)
- `make test-norace` — run tests without race detector
- `make test-verbose` — verbose tests with race detector

Lint:
- `make vet` — run `go vet ./...`
- `make lint` — run `golangci-lint` if installed, else skips
- `make fmt` — run `go fmt ./...`

Run:
- `make run` — build + run locally (requires sudo for /dev access)
- `make dev` — start dev Docker compose with hot reload (air)
- `make dev-detached` — dev compose in background
- `make prod` — production compose (detached)
- `make logs` — tail dev container logs
- `make shell` — shell into dev container

CI (what GitHub Actions runs):
- `go vet ./...`
- `go test ./... -race -count=1`
- `CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/usb-wiper ./cmd/usb-wiper`

## 1. Project at a Glance

USB Wiper securely wipes USB storage devices with multiple wipe schemes (Zero, Random, DoD 3-pass, NIST 800-88 Clear, ATA Secure Erase), verifies results via random-chunk sampling, generates tamper-evident Ed25519-signed certificates (PDF + JSON), and provides a real-time web UI via Server-Sent Events. Runs as a single `--privileged` Docker container.

| Fact | Value |
|------|-------|
| Project kind | Backend server + embedded web frontend |
| Target platform | Linux (amd64) |
| Distribution | Docker Hub (`michealchoudhary/usb-wiper`) |
| Build base | `golang:1.26-alpine` |
| Runtime base | `debian:stable-slim` |
| Language | Go 1.26 (stdlib only — zero external Go modules) |
| Frontend | Vanilla HTML5 + ES modules + CSS custom properties (no frameworks, no build step) |
| Realtime | Server-Sent Events (push-driven, no polling) |
| Container | Single container, `--privileged` required for /dev access |
| License | MIT |

## 2. What This Codebase Deliberately Is NOT

| Removed / Rejected | Replaced By | Why |
|---|---|---|
| External Go modules / dependencies | Standard library only (`os/exec` for system tools) | Minimize supply chain risk for a security tool |
| Frontend frameworks (React, Vue, Svelte) | Vanilla HTML5 + ES modules + CSS custom properties | No build step, zero frontend deps, fast loading |
| Backend frameworks (Gin, Echo, Chi) | `net/http` with Go 1.22+ enhanced routing (`mux.HandleFunc("METHOD /path", ...)`) | Stdlib routing is sufficient; no extra dependency |
| SQLite / any DB | JSON files with atomic writes (temp + rename + dir sync) | Filesystem is the right store for a single-node appliance |
| WebSocket transport | Server-Sent Events | SSE is simpler, unidirectional, auto-reconnects, and sufficient for server→client push |
| Multi-arch Docker builds | Single `linux/amd64` build | Simpler CI, smaller build matrix; USB wiping is a physical-server task |
| Alpine runtime base | `debian:stable-slim` | smartctl/mkfs.vfat/parted/hdparm/nvme are apt packages; Alpine equivalents are unreliable |
| Polling for device changes | 3-second background watcher with SSE push on change | Push > poll for real-time UI responsiveness |
| Config hot-reload (SIGHUP) | File read on startup, `PUT /api/settings` writes to disk + updates in-memory copy | Simpler; no need for in-process reload complexity |

**Rule:** if your task seems to need any of the removed items, stop and re-read the requirements — the stdlib constraint is non-negotiable.

## 3. Technology Stack

| Layer | Technology | Version | Notes |
|-------|-----------|---------|-------|
| Language | Go | 1.26+ | `go 1.26` in go.mod |
| HTTP | `net/http` | stdlib | Go 1.22+ enhanced routing (`mux.HandleFunc("GET /path", ...)`) |
| Templates | `html/template` | stdlib | Not used for UI; index.html is served raw from embedded FS |
| Frontend | ES modules + CSS custom properties | — | No build step; files served from embedded `//go:embed all:static` |
| Realtime | SSE (`text/event-stream`) | — | Custom SSE hub with ring buffer for Last-Event-ID replay |
| Crypto | `crypto/ed25519`, `crypto/sha256`, `crypto/rand` | stdlib | Certificate signing and verification |
| PDF | Pure Go PDF 1.4 writer | stdlib | `internal/cert/pdf.go` |
| ID generation | ULID (stdlib-only) | — | `internal/ulid/ulid.go` |
| Metrics | Prometheus text format | — | `internal/metrics/metrics.go`, served on separate port (`METRICS_BIND`) |
| Shell tools | smartctl, mkfs.vfat, parted, lsblk, hdparm, nvme | apt | Installed in Dockerfile runtime stage |
| Container base | `debian:stable-slim` | — | Runtime |
| Build base | `golang:1.26-alpine` | — | Multi-stage Docker build |
| Dev hot reload | air | latest | `Dockerfile.dev`, `.air.toml` |

## 4. Service / Container Topology

Single-service architecture. One container, one process (`usb-wiper`).

```
┌──────────────────────────────────────────┐
│  usb-wiper container (--privileged)       │
│                                           │
│  Port 8181:  HTTP + SSE (app)             │
│  Port 9090:  /metrics (Prometheus)        │
│                                           │
│  Volumes:                                 │
│    /dev:/dev        (block devices)       │
│    /sys:/sys:ro     (device detection)    │
│    /data            (persistence)         │
│                                           │
│  Background goroutines:                   │
│    - HTTP server (ListenAndServe)         │
│    - Metrics server (loopback)            │
│    - Device watcher (3s ticker)           │
│    - Job queue dispatcher                 │
│    - SSE heartbeat (25s)                  │
└──────────────────────────────────────────┘
```

## 5. Repository Map

```
cmd/usb-wiper/                    → Entry point: env parsing, signal handling, server.New()
internal/
  server/                          → HTTP lifecycle
    server.go                      → Server struct, Start(), mux routing, watchDevices()
    handlers.go                    → All REST + SSE handlers
    sse.go                         → SSEHub: subscribe/broadcast/ring buffer
    middleware.go                  → Logging (ULID per request), panic recovery, CORS
    static/                        → Embedded frontend (//go:embed all:static)
      index.html                   → SPA shell
      css/tokens.css               → CSS custom properties (design tokens)
      css/layout.css               → Page layout
      css/components.css            → Reusable component styles
      js/api.js                    → Fetch wrappers for all API endpoints
      js/state.js                  → Client-side state management
      js/app.js                    → App bootstrap, router, SSE connection
      js/components/drawer.js      → Device detail drawer
      js/components/toast.js       → Toast notification component
      js/views/                    → View controllers (dashboard, devices, history, presets, queue, settings)
  device/                           → Hardware interaction
    detect.go                      → USB device enumeration from /sys/block
    safety.go                      → 9-step safety checks (DO NOT MODIFY)
    safety_other.go                → Safety stub for non-Linux (build tag)
    smart.go                       → SMART health via smartctl multi-bridge
  wipe/                             → Wipe engine
    scheme.go                      → Scheme interface + registry
    scheme_zero.go                 → Zero, Random, DoD, NIST implementations
    scheme_secure_erase.go         → ATA Secure Erase / NVMe Format
    wipe.go                        → Legacy Wipe() + VerifyRandomChunks + progress/speed
    wipe_linux.go                  → Linux build: BLKGETSIZE64 ioctl
    wipe_other.go                  → Non-Linux stub
    progress.go                    → ProgressEvent type
  queue/                            → Job management
    queue.go                       → FIFO queue + concurrency semaphore + dispatch
    queue_helpers.go               → Shared helpers (formatDevice, writeHistory)
    queue_helpers_linux.go         → Linux: openDevice, blkGetSize64 via syscall
    queue_helpers_other.go         → Non-Linux stub
  cert/                             → Certificates of erasure
    cert.go                        → Ed25519 signing, verification, Signer
    pdf.go                         → Pure-Go PDF 1.4 generator
  config/                           → Settings management
    config.go                      → In-memory config + atomic JSON persistence
  persistence/                      → Wipe history
    persistence.go                 → JSON store with atomic writes
  presets/                          → Wipe presets
    presets.go                     → Named reusable configurations
  audit/                            → Audit trail
    audit.go                       → Append-only JSON lines log with rotation
  metrics/                          → Prometheus
    metrics.go                     → Counter + gauge registry
  notify/                           → Webhooks
    notify.go                      → Webhook POST on job completion
  scheduler/                        → Scheduled wipes (PLANNED)
    scheduler.go                   → Stub — cron parser not yet implemented
  ulid/                             → ULID generator
    ulid.go                        → Stdlib-only ULID implementation
Dockerfile                          → Multi-stage production build
Dockerfile.dev                      → Dev build with air hot reload
.air.toml                           → Air config (watches .go/.html/.css/.js)
Makefile                            → All build/test/run targets
deploy/
  docker-compose.dev.yml            → Dev compose (hot reload, /src mount)
  docker-compose.prod.yml           → Prod compose (--privileged, /dev + /sys mounts)
  .env.example                      → Example env vars
  prod.env                          → Production env template
.github/workflows/
  ci.yml                            → Test + vet + build on push/PR
  release.yml                       → Docker build + push to Docker Hub
  auto-tag.yml                      → Auto-version bumping via git-cliff
```

## 6. Lifecycle / App Structure

### Boot Sequence
1. `main()` parses env vars (PORT, LOG_LEVEL, DATA_DIR, UNSAFE_ALLOW_ALL_USB)
2. Creates signal context (SIGINT, SIGTERM)
3. Calls `server.New()` which initializes all subsystems:
   - `persistence.New()` — loads wipe history from disk
   - `presets.New()` — loads presets from disk
   - `wipe.NewSchemeRegistry()` — registers all 5 wipe schemes
   - `SSEHub` — ring buffer + client registry
   - `queue.New()` — FIFO job queue with concurrency semaphore (default 2)
   - `config.New()` — loads settings from disk
   - `cert.NewSigner()` — loads or generates Ed25519 key pair
   - `audit.New()` — opens append-only audit log
   - `metrics.New()` — Prometheus counters/gauges
   - `scheduler.New()` — stub (disabled)
   - Logs `server_start` audit event
4. `srv.Start(ctx)`:
   - Mounts all routes on `http.ServeMux`
   - Applies middleware chain: CORS → Recovery → Logging
   - Starts metrics listener on `METRICS_BIND` (default `127.0.0.1:9090`) in goroutine
   - Starts device watcher in goroutine (3s ticker, SSE broadcast on change)
   - Starts job queue dispatcher in goroutine
   - Starts HTTP server on `:PORT` (default `:8181`)
   - Blocks until context cancellation

### Shutdown Sequence
1. Signal received → context cancelled
2. `http.Server.Shutdown()` with 5s timeout
3. Returns to `main()`, exits

### Key Goroutines (background)
- HTTP server (`ListenAndServe`)
- Metrics server (separate listener, loopback-only by default)
- Device watcher (3s ticker, delta detection, SSE broadcast)
- Job queue dispatcher (blocking on dispatch channel)
- SSE heartbeat (25s keepalive per connection)

## 7. Data Layer / Persistence

All data stored under `$DATA_DIR/` (default `/data/`):

| File | Format | Atomic Write | Purpose |
|------|--------|-------------|---------|
| `history.json` | JSON array | Temp file + rename + dir sync | Wipe history |
| `settings.json` | JSON object | Temp file + rename + dir sync | User settings |
| `presets.json` | JSON array | Temp file + rename + dir sync | Wipe presets |
| `schedules.json` | JSON array | Temp file + rename + dir sync | Schedules (stub) |
| `audit.log` | JSON lines (one per line) | Append + fsync, rotate at 50 MiB | Append-only audit trail |
| `keys/signing.ed25519` | PEM-encoded PKCS#8 (0600) | Direct write | Certificate signing key |
| `keys/signing.ed25519.pub` | PEM-encoded SPKI (0644) | Direct write | Certificate verification key |

**Atomic write pattern:** `os.CreateTemp()` → write → `Sync()` → `Close()` → `os.Rename()` → `dir.Sync()`. This is used by `persistence.save()`, `config.save()`, and `presets.save()`.

**Rules for changing persistence:**
- Never change the JSON shape of existing records without backward-compat handling
- Always use the atomic write pattern (no direct overwrites)
- Audit log is append-only — never modify or delete existing lines
- Keys are loaded/generated once at startup; never regenerated

## 8. Caching / In-Memory State

| Component | What's Cached | Scope | Producer | Consumer |
|-----------|--------------|-------|----------|----------|
| `config.Manager` | Full `Config` struct | Process lifetime | `New()` / `Update()` | Handlers |
| `persistence.Store` | All `WipeRecord` entries | Process lifetime | `load()` on startup / `Append()` | Handlers, queue |
| `presets.Store` | All `Preset` entries | Process lifetime | `load()` on startup / CRUD | Handlers |
| `queue.Queue` | All jobs (active + completed) | Process lifetime | `Enqueue()` / dispatch | Handlers |
| `SSEHub.ring` | Last 100 events | Ring buffer | `Broadcast()` | SSE reconnect via `Last-Event-ID` |
| `SSEHub.clients` | Connected SSE channels | Connection lifetime | `Subscribe()` | SSE handler |
| `device watcher` | Last known device path list | Between ticks | `ListUSBDevices()` | Change detection |

**Rule:** all in-memory caches are read-through on write — when a handler mutates state (e.g., updates settings), it updates the in-memory copy AND persists to disk in the same call.

## 9. API / Network Layer

### Route Table (every endpoint)

| Method | Path | Handler | Notes |
|--------|------|---------|-------|
| GET | `/` | `handleIndex` | Serves embedded `index.html` |
| GET | `/static/` | `http.FileServer` | Embedded static files |
| GET | `/api/devices` | `handleGetDevices` | USB devices with wipe status + history |
| GET | `/api/health` | `handleGetHealth` | Requires `?device=` query param |
| GET | `/api/history` | `handleGetHistory` | Optional `?device=` filter |
| POST | `/api/wipe` | `handlePostWipe` | Accepts `devices` (array) or `device` (legacy single) |
| POST | `/api/test-wipe` | `handlePostTestWipe` | Read-only zero verification |
| POST | `/api/cancel` | `handlePostCancel` | `?device=` cancels specific; no param cancels all |
| GET | `/api/jobs` | `handleGetJobs` | List all jobs |
| GET | `/api/jobs/{id}` | `handleGetJob` | Single job |
| POST | `/api/jobs/{id}/cancel` | `handlePostCancelJob` | Cancel specific job |
| GET | `/api/schemes` | `handleGetSchemes` | Built-in schemes |
| GET | `/api/presets` | `handleGetPresets` | List presets |
| POST | `/api/presets` | `handlePostPresets` | Create preset |
| PUT | `/api/presets/{id}` | `handlePutPreset` | Update preset (partial via pointer fields) |
| DELETE | `/api/presets/{id}` | `handleDeletePreset` | Delete preset |
| GET | `/api/settings` | `handleGetSettings` | Full config |
| PUT | `/api/settings` | `handlePutSettings` | Partial update via `ConfigUpdate` |
| GET | `/api/config` | `handleGetConfig` | Backward compat — same as `/api/settings` |
| POST | `/api/config` | `handlePostConfig` | Backward compat — limited to autoFormat + verifySizeGB |
| GET | `/api/cert/pubkey` | `handleGetPubKey` | Ed25519 public key (base64) |
| GET | `/api/cert/{jobId}/json` | `handleGetCertJSON` | Download JSON certificate |
| GET | `/api/cert/{jobId}/pdf` | `handleGetCertPDF` | Download PDF certificate |
| POST | `/api/cert/verify` | `handlePostCertVerify` | Verify certificate body (1 MiB max) |
| GET | `/api/audit` | `handleGetAudit` | Last 200 audit events (newest first) |
| GET | `/api/schedules` | `handleGetSchedules` | List schedules |
| POST | `/api/schedules` | `handlePostSchedules` | Returns 501 — disabled |
| DELETE | `/api/schedules/{id}` | `handleDeleteSchedule` | Delete schedule |
| GET | `/api/events` | `handleSSE` | SSE stream |
| GET | `/metrics` | Prometheus handler | Separate listener (default `127.0.0.1:9090`) |
| GET | `/healthz` | `handleHealthz` | Liveness probe (returns "ok") |

### Middleware Order (and WHY)

1. **CORS** (outermost): strips OPTIONS preflight requests immediately; for non-OPTIONS, sets CORS headers on local/private origins. Must run first so preflight never reaches recovery/logging.
2. **Recovery**: catches panics, returns 500 JSON. Must be inside CORS so panic responses get CORS headers.
3. **Logging** (innermost): generates ULID per request, sets `X-Request-ID` header, logs method/path/status/duration. Innermost to capture the actual response status code.

### Error Response Convention
```json
{"error": "human-readable message"}
```
HTTP status: 4xx for client errors, 5xx for server errors. All non-trivial errors go through `writeError()`.

### Add-Endpoint Checklist
1. Add handler method on `*Server` in `handlers.go`
2. Register route in `server.go` `Start()` method using `mux.HandleFunc("METHOD /path", s.handleXxx)`
3. Call `s.auditEvent(r, "event_name", target, details)` for security-relevant actions
4. Call `s.sendRefreshEvent()` for actions that change device/job state
5. Return all responses via `writeJSON()` or `writeError()`
6. Read body with `json.NewDecoder(r.Body).Decode()` (use `http.MaxBytesReader` for untrusted uploads like cert verify)

## 10. SSE Protocol

### Hub Architecture (`sse.go`)
- Ring buffer of last 100 events with monotonic IDs (atomic counter)
- Subscribe/Unsubscribe pattern (buffered channel, cap 64)
- Broadcast writes to ring buffer first, then fans out to live clients
- `Last-Event-ID` replay: client sends header, hub replays `EventsSince(lastID)`
- On fresh connect without `Last-Event-ID`: sends current running job states
- 25s heartbeat keepalive (SSE comment line `: keepalive\n\n`)
- Client buffer full → event dropped with log warning

### Event Types

| Event Type | When Sent | Data Fields |
|-----------|-----------|-------------|
| `"refresh"` | Wipe started/completed/cancelled, device plugged/unplugged | `devicePath`, `status`, `timestamp` |
| `"job"` | Job state change (queued → running → complete) | `devicePath`, `status`, `percent`, `currentPass`, `totalPasses`, `timestamp` |
| (unset) | Wipe/verification progress | `devicePath`, `status`, `percent`, `bytesWritten`, `totalBytes`, `currentPass`, `totalPasses`, `speed`, `eta`, `message` |

### SSE Headers
```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no
```

## 11. External Integrations

### Shell Commands (via `os/exec`)

| Tool | Purpose | Where Used |
|------|---------|------------|
| `smartctl -i -j` | Device identity (model, serial) | `device/smart.go` → `GetSmartIdentity()` |
| `smartctl -a -j` | Full SMART health | `device/smart.go` → `GetHealth()` |
| `parted -s ... mklabel msdos` | Partition table creation | `format/format.go` → `FormatFAT32()` |
| `parted -s ... mkpart primary fat32` | FAT32 partition creation | `format/format.go` |
| `mkfs.vfat -F 32` | FAT32 filesystem | `format/format.go` |
| `hdparm --user-master u --security-set-pass` | ATA secure erase unlock | `wipe/scheme_secure_erase.go` |
| `hdparm --user-master u --security-erase` | ATA secure erase | `wipe/scheme_secure_erase.go` |
| `nvme format` | NVMe format | `wipe/scheme_secure_erase.go` |
| `lsblk -J` | Block device info | `wipe/scheme_secure_erase.go` |

All commands use `context.WithTimeout()` with appropriate deadlines. `smartctl` tries multiple `-d` flags (auto, `sat,12`, `sat,16`, `sntasmedia`, `sntjmicron`, `sntrealtek`) to handle USB bridge chips.

### Webhook Notifications
- `POST` to configured `webhookUrl` on each job completion
- URL validated against private/internal IPs unless `UNSAFE_ALLOW_PRIVATE_WEBHOOKS=1`
- Payload: job ID, device, status, verification result, timestamps

## 12. The Critical Path: Wipe Job Lifecycle

This is the most complex flow — a wipe job from enqueue to completion:

1. **Enqueue** (`POST /api/wipe` → `queue.Enqueue()`)
   - Resolve device list (support both `devices` array and legacy `device` string)
   - Resolve preset (if `presetId` provided) → inherit scheme/autoFormat/verifySizeGB/label
   - For each device:
     - Run `device.IsSafeToWipe()` (all 9 checks)
     - Check device not already active/queued (`ErrJobAlreadyActive`)
     - Validate scheme exists
     - Create `Job` with ULID, status=queued, totalPasses from scheme
     - Add to pending FIFO, signal dispatcher
   - Call `sendRefreshEvent()` → SSE broadcast
   - Return 202 with started + conflicts lists

2. **Dispatch** (`queue.dispatch()` — goroutine)
   - Check: pending queue not empty AND active count < maxParallel
   - Pop first pending job, add to `byDevice` map (active devices)
   - Start `runJob()` in goroutine

3. **Run** (`queue.runJob()` — goroutine)
   - Acquire concurrency sem slot
   - **RE-VALIDATE SAFETY** before destruction (non-negotiable)
   - Set status=running, create cancel context, start progress channel reader
   - Get device size via `BLKGETSIZE64` ioctl
   - Execute wipe scheme → pipe progress to SSE
   - On scheme error → `failJob()` (status=failed, persist, broadcast, signal dispatch)

4. **Verification** (if `verifySizeGB > 0`)
   - **RE-VALIDATE SAFETY** before reading device
   - Set status=verifying, broadcast
   - Run `wipe.VerifyRandomChunks()` — reads random 1 MiB chunks across device, checks all bytes are zero
   - Pipe verification progress to SSE
   - On non-zero byte found → fail with offset detail

5. **Format** (if `autoFormat`)
   - **RE-VALIDATE SAFETY** before partitioning
   - Set status=formatting, broadcast
   - Run `format.FormatFAT32()` (which also re-checks safety internally)
   - On format failure → fail but note "Wipe completed but format failed"

6. **Complete**
   - Set status=completed, progress=100, persist to history, broadcast job event
   - Send final `"refresh"` SSE event with verification result
   - Release sem slot, signal dispatcher for next job

### Cancellation
- `Cancel(jobID)`: if queued, remove from pending; if running, cancel context; set status=cancelled
- `CancelDevice(devicePath)`: finds active/pending job for device, delegates to Cancel
- `CancelAll()`: cancels all active/pending jobs, returns count

## 13. Auth / Security Model

> **Note:** Auth is currently stubbed (`AuthEnabled` and `AuthToken` fields exist in config but no middleware enforces them). The UI settings page stores these but they have no effect. The project is expected to be run on trusted local networks with no authentication.

### Safety Rules (NON-NEGOTIABLE)
1. Safety checks in `internal/device/safety.go` MUST NOT be modified, reordered, simplified, or weakened
2. `IsSafeToWipe()` MUST be called before EVERY destructive operation: wipe, verify, format, secure erase — not just at enqueue time but also in the queue worker right before execution
3. Never accept user-provided device paths without server-side re-validation
4. Never log device contents
5. No external Go dependencies (stdlib only)

### Safety Check Order (as implemented)
1. Path regex: `^/dev/sd[a-z]$` (no partitions, no NVMe, no loop, no dm-*)
2. `os.Stat` exists
3. Mode is block device (`ModeDevice`)
4. Not containing "nvme"
5. Not the root device (parsed from `/proc/mounts`)
6. `removable == 1` (skipped when `UNSAFE_ALLOW_ALL_USB=1`)
7. USB bus (`/usb` in resolved device symlink via `filepath.EvalSymlinks`)
8. Not mounted at system paths: `/`, `/boot`, `/home`, `/var`, `/usr`, `/etc`
9. Size ≤ 2 TB (read from `/sys/block/<name>/size` * 512)

## 14. Observability

### Logging
- Standard library `log` with `LstdFlags | Lmicroseconds`
- Per-request ULID correlation ID (`X-Request-ID` header + `ctxRequestID` context key)
- Format: `[ULID] METHOD /path STATUS DURATION`
- Audit events go to `audit.Logger` (separate from application logs)

### Metrics (Prometheus)
| Name | Type | Description |
|------|------|-------------|
| `usb_wiper_wipes_total` | Counter | Total wipe jobs started |
| `usb_wiper_wipes_completed` | Counter | Jobs completed successfully |
| `usb_wiper_wipes_failed` | Counter | Jobs that failed |
| `usb_wiper_jobs_running` | Gauge | Currently running jobs |
| `usb_wiper_jobs_queued` | Gauge | Queued jobs |

Served on separate listener (`METRICS_BIND`, default `127.0.0.1:9090`). Set to `off` to disable.

### Audit Log
- Append-only JSON lines in `audit.log`
- 50 MiB rotation (rename to `audit-YYYYMMDD-HHmmss.log`)
- Events include: `server_start`, `wipe_started`, `wipe_completed`, `wipe_cancelled`, `settings_updated`, `preset_created/updated/deleted`
- Read via `GET /api/audit` (newest first, max 200)

### Health Check
- `GET /healthz` returns 200 "ok"
- Docker HEALTHCHECK: `wget -qO- http://localhost:8181/healthz` every 30s

## 15. Configuration

### Environment Variables

| Variable | Required? | Default | Notes |
|----------|-----------|---------|-------|
| `PORT` | No | `8181` | HTTP port |
| `LOG_LEVEL` | No | `info` | Currently only controls log verbosity via env var logging; not runtime-queryable |
| `DATA_DIR` | No | `/data` | Persistence root |
| `METRICS_BIND` | No | `127.0.0.1:9090` | Set to `off` to disable metrics listener entirely |
| `UNSAFE_ALLOW_ALL_USB` | No | (unset) | Skip removable flag check (safety check #6) |
| `ALLOW_HARDWARE_SECURE_ERASE` | No | (unset) | Enable ATA Secure Erase / NVMe Format scheme |
| `UNSAFE_ALLOW_PRIVATE_WEBHOOKS` | No | (unset) | Allow webhook URLs to private/internal IPs |

### Runtime Config (persisted to `settings.json`)
Configurable via `PUT /api/settings`:

| Field | Type | Default |
|-------|------|---------|
| `autoFormat` | bool | `false` |
| `verifySizeGB` | int | `1` |
| `maxParallelJobs` | int | `2` |
| `defaultSchemeId` | string | `"zero"` |
| `defaultPresetId` | string | `""` |
| `theme` | string | `"dark"` |
| `notificationsOn` | bool | `false` |
| `webhookUrl` | string | `""` |
| `authEnabled` | bool | `false` |
| `authToken` | string | `""` |
| `historyRetentionDays` | int | `0` (forever) |

### Build-time Config
- `version` and `buildTime` set via `-ldflags="-X main.version=... -X main.buildTime=..."` in Docker build
- Default: `"dev"` and `"unknown"`

## 16. Conventions & Idioms

### Go
- **Naming:** standard Go conventions — exported types PascalCase, unexported camelCase, short variable names in small scopes
- **File organization:** one package per directory, one concern per file (but handlers.go is large because all handlers are on *Server)
- **Imports:** stdlib only, grouped as: stdlib first, then internal packages
- **Error handling:** return errors upward, wrap with `fmt.Errorf("context: %w", err)`, never panic except in `SchemeRegistry.Register()` for duplicate IDs (programmer error)
- **Logging:** use `log.Printf` with context; avoid `log.Fatal` — return errors to caller
- **Concurrency:** `sync.Mutex`/`sync.RWMutex` for shared state; channels for SSE fan-out; `atomic` for monotonic counters
- **Context:** pass `context.Context` to all long-running operations; respect cancellation
- **Build tags:** `//go:build linux` for safety.go, wipe_linux.go, queue_helpers_linux.go; `//go:build !linux` for stubs
- **Embed:** `//go:embed all:static` for frontend files

Example of good Go style (from `audit.go`):
```go
func (l *Logger) Log(ev Event) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now()
	}

	data, err := json.Marshal(ev)
	if err != nil {
		return fmt.Errorf("marshal audit event: %w", err)
	}
	// ...
}
```

### Frontend
- **Naming:** ES module imports use relative paths (`'../api.js'`); functions `camelCase`; custom elements with hyphens
- **CSS:** custom properties defined in `css/tokens.css` for theming (`--color-bg`, `--color-text`, etc.); dark/light themes swap property values
- **State management:** `js/state.js` exports a simple observable pattern; views subscribe and re-render on change
- **API calls:** `js/api.js` centralizes all fetch calls, handles errors uniformly, returns JSON
- **SSE:** `js/app.js` establishes `EventSource` connection, dispatches named events to views
- **No build step:** ES modules work natively in browsers, CSS custom properties are natively supported, no transpilation needed

Example of good frontend pattern (from `js/api.js`):
```js
export async function fetchDevices() {
  const res = await fetch('/api/devices');
  if (!res.ok) throw new Error(await res.text());
  return res.json();
}
```

### Commit Messages
- Conventional commits: `feat:`, `fix:`, `chore:`, `docs:`, `ci:`
- Format: `type: description` (lowercase, no period)
- Example: `fix(queue): return Job copies from Get and List to prevent data race`

## 16b. BOUNDARIES

### ✅ Always Do
- Write new Go code in `internal/<package>/` following the existing pattern
- Write frontend code in `internal/server/static/js/` and `internal/server/static/css/`
- Run `make vet` before considering any change ready
- Run `make test` (with race detector) before committing
- Call `device.IsSafeToWipe()` before every destructive operation — including re-validation in the queue worker
- Use `writeJSON()` or `writeError()` for all API responses — never write raw JSON manually
- Use `sendRefreshEvent()` after any action that changes device/job state
- Use the atomic write pattern (temp file → write → sync → close → rename → dir sync) for all persisted data
- Add audit events for security-relevant actions via `s.auditEvent(r, ...)`
- Follow existing file patterns: one package per dir, handlers on `*Server` struct methods, SSE broadcasts go through `sseHub.Broadcast()`
- Use build tags (`//go:build linux`) for platform-specific code (device IO, syscalls)
- Add Go-style context cancellation support to any new long-running operations
- Handle the `Job` copy pattern: `Get()` and `List()` return copies to prevent data races (see v0.5.2 fix)
- Set `WriteTimeout: 0` on the main HTTP server (SSE requires unbuffered streaming)
- Use `debug/stack` in `withRecovery` to log panics with full stack traces

### ⚠️ Ask First
- Major rewrites of existing files (>50 lines in handlers.go, server.go, queue.go)
- Changing the JSON shape of `WipeRecord`, `Config`, `Preset`, `Schedule`, or `Certificate` structs
- Adding new environment variables or changing defaults of existing ones
- Modifying the Dockerfile base image or installed apt packages
- Changing the wipe scheme interface (`Scheme` in `scheme.go`)
- Modifying the SSE event protocol (event type names, field names, wire format)
- Changing the atomic write pattern or persistence file format
- Adding new shell tool dependencies (anything beyond the current smartctl/mkfs.vfat/parted/hdparm/nvme/lsblk set)
- Enabling the scheduler (currently stubbed — cron parser is incomplete)
- Implementing auth enforcement (config fields exist but no middleware enforces them)

### 🚫 Never Do
- Modify, weaken, reorder, or remove any safety check in `internal/device/safety.go` — the 9 checks are exhaustive and ordered for a reason
- Add external Go module dependencies — stdlib only, non-negotiable
- Add frontend framework dependencies (React, Vue, etc.) — vanilla HTML/CSS/JS only
- Add build steps for the frontend (Webpack, Vite, etc.) — ES modules + CSS custom properties work natively
- Accept user-provided device paths without server-side `IsSafeToWipe()` re-validation
- Log device contents or raw block data
- Force-push to the main branch
- Reintroduce multi-arch Docker builds (removed in v0.1.0 for simplicity)
- Reintroduce Alpine as the runtime base (switched to Debian for apt package access)
- Reintroduce WebSocket transport (SSE is the chosen push protocol)
- Edit generated or auto-managed files: `go.sum`, `cliff.toml` (generated), `renovate.json`
- Change the module path (`github.com/usb-wiper`)
- Remove or rename the `UNSAFE_ALLOW_ALL_USB` flag — backward compatibility with existing deployments is expected
- Skip `make vet` and `make test` in CI (both are enforced in `.github/workflows/ci.yml`)

## 17. Common Tasks

### Add a New API Endpoint
1. Add handler method on `*Server` in `internal/server/handlers.go`:
   ```go
   func (s *Server) handleGetFoo(w http.ResponseWriter, r *http.Request) {
       writeJSON(w, http.StatusOK, map[string]interface{}{"foo": "bar"})
   }
   ```
2. Register route in `server.go` `Start()` method:
   ```go
   mux.HandleFunc("GET /api/foo", s.handleGetFoo)
   ```
3. For mutating endpoints: add audit event via `s.auditEvent(r, "event_name", target, details)`
4. For endpoints that change device/job state: call `s.sendRefreshEvent()`
5. Add corresponding fetch wrapper in `internal/server/static/js/api.js`
6. Update UI view in `internal/server/static/js/views/` if needed

### Add a New Wipe Scheme
1. Create `internal/wipe/scheme_<name>.go` implementing the `Scheme` interface:
   ```go
   type SchemeFoo struct{}
   func (s *SchemeFoo) ID() string { return "foo" }
   func (s *SchemeFoo) DisplayName() string { return "Foo Wipe" }
   func (s *SchemeFoo) Passes() int { return 1 }
   func (s *SchemeFoo) Execute(ctx context.Context, devicePath string, size uint64, progress chan<- ProgressEvent) error {
       // Implementation
   }
   ```
2. Register in `NewSchemeRegistry()` in `internal/wipe/scheme.go`:
   ```go
   r.Register(&SchemeFoo{})
   ```
3. Optionally add to `schemeDescription()` in `scheme.go`

### Add a New Frontend View
1. Create `internal/server/static/js/views/<name>.js`
2. Export an `init()` and `destroy()` function; import state from `'../state.js'`, API from `'../api.js'`
3. Wire up in `js/app.js` routing logic
4. Add CSS for the view in `css/components.css` (or a dedicated file)
5. Use existing CSS custom properties from `tokens.css` — do not hardcode colors

### Add a New Persisted Record Type
1. Copy the atomic write pattern from `persistence.go` or `presets.go`:
   - `os.CreateTemp(dir, prefix)` → `Write()` → `Sync()` → `Close()` → `os.Rename(tmp, dest)` → `dir.Sync()`
2. Use `sync.RWMutex` for thread-safe access
3. Load on startup, save on every mutation
4. Handle empty/missing file gracefully (return empty collection, not error)

### Add a New Config Field
1. Add field to `Config` struct in `config/config.go`
2. Add pointer field to `ConfigUpdate` struct (for partial PATCH support)
3. Add `if updates.Foo != nil` case in `Manager.Update()`
4. Set default in `New()`
5. Update `PUT /api/settings` handler if validation is needed
6. Update frontend settings view with the new field

## 18. Testing

### Commands
```bash
make test          # go test ./... -race -count=1
make test-norace   # go test ./... -count=1 (CI doesn't use race on Linux)
make test-verbose  # go test -v ./... -race -count=1
```

### Test Files
| File | Tests |
|------|-------|
| `internal/device/safety_test.go` | Safety check validation |
| `internal/device/detect_test.go` | Device detection (limited — requires real /sys) |
| `internal/device/smart_test.go` | SMART parsing |
| `internal/wipe/wipe_test.go` | VerifyRandomChunks |
| `internal/queue/queue_test.go` | Queue operations |
| `internal/persistence/persistence_test.go` | Store CRUD |
| `internal/presets/presets_test.go` | Preset CRUD |
| `internal/cert/cert_test.go` | Certificate signing/verification |
| `internal/ulid/ulid_test.go` | ULID generation |
| `internal/notify/notify_test.go` | Webhook URL validation |

### Testing Rules
- Tests that interact with `/sys/block`, `/dev/*`, or shell out to `smartctl` use build tags or skip on non-Linux
- `VerifyRandomChunks` tests use a temp file instead of a real device
- Queue tests run with in-memory state (no real device IO)
- Always run with `-race` locally (the server uses goroutines heavily)
- CI runs: `go vet ./...` then `go test ./... -race -count=1`
- The race detector has caught real bugs (v0.5.2: `Job` copies in `Get`/`List`)

## 19. Debugging Tips

| Symptom | Likely Cause |
|---------|-------------|
| SSE connection drops immediately | `WriteTimeout` is set on the HTTP server — must be `0` for SSE |
| SSE progress stops mid-wipe | Client buffer full (cap 64); check for slow consumers or add backpressure |
| Device not showing in UI | Not on a USB bus — check `/sys/block/<name>/device` resolves via `EvalSymlinks` to a path containing `/usb` |
| "removable flag" safety error on USB SSD | UASP enclosures often report `removable=0` — set `UNSAFE_ALLOW_ALL_USB=1` |
| smartctl returns empty results | Wrong `-d` flag for the USB bridge chip — the multi-bridge fallback loop tries all known types |
| "device not found" on reconfigured hardware | Device path changed between plug cycles — `/dev/sdb` may become `/dev/sdc` after re-plug |
| Panic in handler | Recovery middleware catches it — stack trace logged; returns 500 JSON |
| Data race on `Job` fields | Are you returning a pointer to the map entry without copying? `Get()` and `List()` must return copies |
| Write timeout on large wipes | Main server `WriteTimeout` is `0` (unbounded) — only SSE connections timeout; wipe IO is on goroutines, not the HTTP handler |
| Container can't access new USB devices after start | Missing `--privileged` flag in docker run/docker-compose |
| History/settings not persisting across restarts | `DATA_DIR` not mounted as volume or not set correctly |
| Certificate PDF generation fails | Check that the job has completed (status=completed/failed); certificates unavailable for queued/running jobs |

## 20. Known Gaps & Planned Work

| Gap | Location | Notes |
|-----|----------|-------|
| Scheduler disabled | `internal/scheduler/scheduler.go` | `POST /api/schedules` returns 501; cron parser is a non-functional stub that fires every minute. A real 5-field cron parser is planned. |
| Auth not enforced | `internal/config/config.go` + handlers | `authEnabled`/`authToken` fields exist in config and settings UI, but no middleware checks them. Expected to run on trusted local networks. |
| No history retention enforcement | `internal/persistence/persistence.go` | `historyRetentionDays` field exists in config but is not used to prune old records. |
| Metrics not updated by queue | `internal/metrics/metrics.go` + `internal/queue/queue.go` | Counters and gauges are declared but never incremented/set by the queue worker. |
| `schedule_test.go` is empty | `internal/scheduler/` | No tests for the scheduler package. |
| No end-to-end/integration tests | — | All tests are unit tests; no test that spins up the full server or runs a real wipe on a loop device. |
| No download progress for certificates | `internal/cert/pdf.go` | PDF is generated in-memory and buffered before response; could be streamed for large certificates. |
| Hardware secure erase not tested | `internal/wipe/scheme_secure_erase.go` | Requires real hardware with ATA/NVMe support; CI cannot test. |

## 21. Useful URLs / Dev Tooling

| Service | URL | Notes |
|---------|-----|-------|
| Web UI | `http://localhost:8181` | Main application |
| API base | `http://localhost:8181/api/` | All REST endpoints |
| SSE stream | `http://localhost:8181/api/events` | Real-time events |
| Health check | `http://localhost:8181/healthz` | Liveness probe (returns "ok") |
| Prometheus metrics | `http://localhost:9090/metrics` | Separate listener (loopback-only by default) |
| Docker Hub | `michealchoudhary/usb-wiper:latest` | Published image |

## 22. Files to Read Before Any Non-Trivial Change

| File | Why |
|------|-----|
| `internal/device/safety.go` | Understand the 9 safety checks — you MUST NOT modify these but MUST understand when and why they're called |
| `internal/server/server.go` | Understand the full startup sequence, route registration, goroutine lifecycle, and all subsystem initialization |
| `internal/queue/queue.go` | Understand the job lifecycle: enqueue → dispatch → run → verify → format → complete, including all re-validation points |
| `internal/server/handlers.go` | Understand all API handler patterns, request/response shapes, and audit/SSE integration |
| `internal/wipe/scheme.go` | Understand the `Scheme` interface and registry — every wipe method implements this contract |
| `internal/config/config.go` | Understand config persistence pattern, `ConfigUpdate` partial update semantics, and atomic writes |
| `internal/server/sse.go` | Understand the SSE hub, ring buffer, replay protocol, and broadcast semantics |
| `internal/server/middleware.go` | Understand middleware order (WHY CORS must be outermost), `responseWriter` wrapper, and ULID correlation IDs |
| `Dockerfile` | Understand the multi-stage build: Go build stage → Debian runtime stage with apt packages |
| `Makefile` | Understand all available targets and their exact commands |

---

## Changelog (this document)

### v2.0 — 2026-05-23 — Complete rewrite
- Replaced the entire document with a comprehensive, code-verified AGENTS.md
- Every section driven by actual code inspection, not transcription of the old spec
- Added: Section 0a (YAML front-matter + role), 0b (role block), 0c (verified commands), 4 (container topology), 6 (lifecycle/app structure with goroutine map), 7 (data layer with atomic write pattern), 8 (in-memory state), 10 (SSE protocol details including ring buffer + replay), 11 (external integrations with shell tool table), 12 (critical path: full job lifecycle with all re-validation points), 13 (auth model with safety rules), 16b (boundaries with concrete paths and actions), 17 (common tasks with code snippets), 18 (testing with test file map), 19 (debugging tips from real gotchas), 20 (known gaps), 21 (useful URLs), 22 (files to read)
- Corrections from code inspection:
  - Persistence is JSON array (not JSON lines as old spec said for history.json — only audit.log is JSON lines)
  - `go.mod` is `go 1.26`, not `1.26+` with a plus
  - `Dockerfile.dev` is the dev build file, not mentioned in old spec
  - `SAFETY_OTHER.GO` exists as non-Linux stub (old spec missed build-tag files)
  - `QUEUE_HELPERS_LINUX.GO` / `QUEUE_HELPERS_OTHER.GO` are separate files (old spec pointed to a single `queue_helpers.go`)
  - Scheduler is explicitly stubbed (`POST /api/schedules` returns 501) — old spec listed it as functional
  - Metrics are declared but NOT updated by queue — old spec implied they were functional
  - Auth is NOT enforced — old spec listed auth as functional
  - `schemeDescription()` is defined inline in `scheme.go`, not exported

### Verified against
- All 38 Go source files
- Dockerfile + Dockerfile.dev
- Both compose files
- Makefile
- .air.toml
- CI workflow (`.github/workflows/ci.yml`)
- go.mod
- README.md + CHANGELOG.md
- git log (last 20 commits)

### Could NOT verify (please confirm)
- The `LOG_LEVEL` env var: it's parsed in `main()` but I couldn't find where it actually controls log verbosity at runtime. It may be logged but unused.
- The exact shape of `notify.go` webhook payload: I didn't read the full file.
- Preset store save pattern: assumed same atomic write as persistence/config but didn't verify line-for-line.
- Scheduler `DeleteSchedule` endpoint: handler exists but since scheduler is stubbed, it may not work correctly.