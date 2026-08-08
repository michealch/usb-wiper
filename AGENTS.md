# AGENTS.md

This file is the agent-facing guide for USB Wiper. It complements `README.md`
with the project constraints, commands, and safety rules needed to make code
changes confidently.

## Project Overview

USB Wiper is a Linux-focused secure device wiping appliance. It runs as one
Go HTTP server with an embedded vanilla web UI, normally inside a privileged
Docker container with access to host `/dev` and `/sys`.

Core capabilities:

- Detect USB-attached block devices from `/sys/block`.
- Identify the physical disk behind USB bridges using SMART WWN/model/serial
  data when available, not the USB enclosure identity.
- Queue destructive wipe jobs with multiple schemes: zero, random, DoD
  3-pass, NIST clear, and firmware secure erase.
- Persist wipe history, settings, presets, SMART health history, auto-wipe
  seen-device state, audit logs, and signing keys under `DATA_DIR`.
- Serve a browser UI from `internal/server/static` using native ES modules,
  CSS custom properties, and Server-Sent Events.

Important architectural choices:

- Go stdlib only. Do not add external Go modules.
- No frontend framework, bundler, CDN, or transpiler.
- No SQL database. Local state is JSON files with atomic writes.
- Server push uses SSE, not WebSockets.
- Runtime image is Debian stable-slim because required disk tools are apt
  packages.

## Setup Commands

All targets run inside a container — the host only needs Docker (no Go toolchain
or Node required). Toolchain image defaults to `golang:1.26`; override with
`GO_IMAGE=...`.

- Build the binary (into `bin/`): `make build`
- Run the appliance (containerized, host `/dev` + `/sys` mounted):
  `make run` — add `UNSAFE_ALLOW_ALL_USB=1` to the command line when needed
- Debug the appliance (dlv headless on :2345): `make debug`
- Build Docker image: `make docker-build`
- Start dev compose with hot reload: `make dev`
- Start dev compose detached: `make dev-detached`
- Start prod compose detached: `make prod`
- Tail dev logs: `make logs`
- Shell into dev container: `make shell`
- Clean local build artifacts: `make clean`

The app listens on `PORT` (default `8181`). Open `http://localhost:8181`.

## Testing Instructions

Run relevant checks before finishing code changes. Every command runs in a
container (`golang:1.26`, Debian — the race detector needs cgo, which Alpine
lacks):

- Fast test pass: `make test-norace`
- Full test pass (race detector): `make test`
- Verbose full pass: `make test-verbose`
- Vet: `make vet`
- Format Go: `make fmt`
- Optional linter: `make lint` (runs `go vet`; runs `golangci-lint` in a
  throwaway container)

CI-equivalent core checks:

```bash
docker run --rm -v "$PWD":/src -w /src \
  -v "$HOME/.cache/go-docker":/root/.cache/go-build \
  golang:1.26 go vet ./...
docker run --rm -v "$PWD":/src -w /src \
  -v "$HOME/.cache/go-docker":/root/.cache/go-build \
  golang:1.26 go test ./... -race -count=1 -coverprofile=coverage.out
CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/usb-wiper ./cmd/usb-wiper  # inside a container
```

JavaScript has no build step. For edited ES modules, run syntax checks with the
host `node` (or `node:22` container):

```bash
node --check internal/server/static/js/app.js
node --check internal/server/static/js/views/wipe.js
node --check internal/server/static/js/components/overlay.js
node --check internal/server/static/js/components/tabs.js
```

Test locations:

- Device safety/detection/SMART: `internal/device/*_test.go`
- Wipe verification and schemes: `internal/wipe/*_test.go`
- Queue behavior: `internal/queue/*_test.go`
- JSON persistence stores: `internal/persistence/*_test.go`
- Presets: `internal/presets/*_test.go`
- Certificates: `internal/cert/*_test.go`
- ULIDs: `internal/ulid/*_test.go`

Some device tests skip or are limited on non-Linux hosts. Real wipe behavior
requires Linux block devices and should be verified in Docker or on hardware.

## Development Workflow

Main entry points:

- `cmd/usb-wiper/main.go`: env parsing, signal handling, `server.New`.
- `internal/server/server.go`: server setup, route registration, background
  device watcher, metrics listener, queue startup.
- `internal/server/handlers.go`: REST endpoints and server-side helpers.
- `internal/server/sse.go`: SSE hub and event replay.
- `internal/server/static/index.html`: SPA shell.
- `internal/server/static/js/app.js`: router, SSE connection, top-level UI glue.
- `internal/server/static/js/views/`: individual UI screens — `wipe.js`,
  `records.js` (History/Activity tabs), `history.js`, `activity.js`,
  `settings.js` (General/Security tabs), `autowipe.js`, `presets.js`.
- `internal/server/static/js/components/`: shared UI pieces — `drawer.js`
  (device inspection), `configurator.js` (batch wipe flow), `hold-confirm.js`
  (hold-to-confirm gate with reduced-motion/keyboard fallback), `cert.js`
  (certificate download/verify), `toast.js`, `overlay.js` (shared overlay
  behaviour: focus trap, restore, Escape/backdrop close), and `tabs.js`
  (tab widget used by the drawer, Records, and Settings).
- `internal/server/static/js/util.js`: shared helpers (`escapeHtml`,
  `escapeAttr`, `formatBytes`, `formatDateTime`, `prefersReducedMotion`,
  `badgeClassForStatus`, `deviceStatusBadge`, `resolveProgress`,
  `segmentWidths`, `renderPassSegments`, `progressBar`, `emptyState`,
  `activeJobByPath`, `schemeOptions`, `verifySizeOptions`).
- `internal/server/static/css/`: `tokens.css` (design tokens — colors, type,
  spacing, glows, motion), `layout.css` (shell/chrome), `components.css`.

When adding a route:

1. Add a `*Server` handler in `internal/server/handlers.go`.
2. Register it in `Start()` in `internal/server/server.go` with
   `mux.HandleFunc("METHOD /path", s.handleX)`.
3. Use `writeJSON()` and `writeError()` for responses.
4. Add audit events for security-relevant mutations.
5. Call `s.sendRefreshEvent()` when device/job state changes.
6. Update the UI route/view when needed.

When adding persisted state:

1. Put the store in a focused package, usually `internal/persistence`.
2. Use `sync.RWMutex`.
3. Load missing or empty files as an empty store.
4. Save with the atomic pattern: temp file, write, `Sync`, `Close`, `Rename`,
   directory `Sync`.
5. Keep JSON backward compatible.

## Repository Map

```text
cmd/usb-wiper/                  process entry point
internal/audit/                 append-only audit log
internal/cert/                  Ed25519 certificates and pure-Go PDF output
internal/config/                persisted runtime settings
internal/device/                USB discovery, identity, safety checks, SMART
internal/format/                FAT32 formatting helpers
internal/jsonfile/              atomic JSON file read/write shared by all stores
internal/persistence/           wipe history, SMART history, auto-wipe state
internal/presets/               named wipe presets
internal/queue/                 FIFO wipe job queue and worker lifecycle
internal/server/                HTTP server, handlers, middleware, SSE
internal/server/static/         embedded frontend assets
internal/ulid/                  stdlib-only ULID generator
internal/wipe/                  wipe schemes and verification logic
deploy/                         Docker Compose files and env examples
.github/workflows/              CI, release, and tagging workflows
```

## Runtime Data

All local app data is under `DATA_DIR` (default `/data`):

| File | Purpose |
| --- | --- |
| `history.json` | Wipe history, including device identity fields |
| `health-history.json` | SMART/health snapshots keyed by physical device ID |
| `auto-wipe-seen.json` | Physical device IDs already observed by Auto Wipe |
| `settings.json` | Runtime settings, including `autoWipeEnabled` |
| `presets.json` | Wipe presets |
| `audit.log` | Append-only JSONL audit trail |
| `keys/signing.ed25519` | Certificate signing private key |
| `keys/signing.ed25519.pub` | Certificate verification public key |

Do not introduce SQLite or another database. This is a single-node appliance
and local JSON persistence is intentional.

## Device Identity and Auto Wipe

Device identity must represent the physical disk, not the USB adapter. The
current preference order is:

1. SMART WWN: high confidence.
2. SMART model + serial + capacity: high confidence.
3. sysfs model + serial + capacity: medium confidence.
4. attachment/diskseq fallback: low confidence.

History and SMART snapshots should only be attached automatically for high or
medium confidence identities. Low-confidence identities are intentionally not
used for cross-attachment history.

Auto Wipe rules:

- `autoWipeEnabled` defaults to `false`.
- Enabling Auto Wipe first marks currently connected trusted-serial devices as
  seen. Enabling the feature must not wipe already attached drives.
- Only later attached devices with trusted identity confidence and a non-empty
  serial are eligible.
- Auto Wipe uses the current default scheme, `autoFormat`, and `verifySizeGB`.
- Normal safety checks still run before queueing and again in the queue worker.
- Record every queued, skipped, conflicted, or errored decision in
  `auto-wipe-seen.json` so the same serial does not loop every watcher tick.

## API Surface

Important endpoints:

- `GET /api/devices`
- `GET /api/health?device=/dev/sdX`
- `GET /api/health-history?deviceId=...`
- `GET /api/history` and `GET /api/history?deviceId=...`
- `POST /api/wipe`
- `POST /api/cancel`
- `GET /api/jobs`
- `POST /api/jobs/{id}/cancel`
- `GET /api/schemes`
- `GET/POST/PUT/DELETE /api/presets`
- `GET/PUT /api/settings`
- `GET /api/autowipe`
- `PUT /api/autowipe`
- `DELETE /api/autowipe/seen`
- `GET /api/cert/pubkey`
- `GET /api/cert/{jobId}/json`
- `GET /api/cert/{jobId}/pdf`
- `POST /api/cert/verify`
- `GET /api/audit`
- `GET /api/events`
- `GET /healthz`

There are no live scheduled-wipe routes in the server UI/API.

## Safety Rules

Never weaken destructive safety. These rules are non-negotiable:

- Do not modify, reorder, weaken, or bypass checks in `internal/device/safety.go`.
- Call `device.IsSafeToWipe()` before every destructive operation: wipe,
  firmware erase, verify, and format.
- Re-validate in workers immediately before device IO; enqueue-time validation
  alone is not enough.
- Never trust a device path from the browser without server-side validation.
- Never log raw device contents or block data.
- Keep `UNSAFE_ALLOW_ALL_USB` backward compatible; it only skips the removable
  flag check for USB SSD/enclosure deployments.
- Secure erase remains gated by `ALLOW_HARDWARE_SECURE_ERASE=1`.

Safety check summary:

1. Path must be `/dev/sd[a-z]`.
2. Device must exist and be a block device.
3. Device must not be NVMe or the root device.
4. Device must be removable unless `UNSAFE_ALLOW_ALL_USB=1`.
5. Device must be on a USB bus.
6. Device must not be mounted at system paths.
7. Device size must be 2 TB or less.

## Code Style

Go:

- Use standard Go formatting: `gofmt` via `make fmt` (runs in the toolchain
  container).
- Keep imports stdlib first, then internal packages.
- Return errors with context using `fmt.Errorf("context: %w", err)`.
- Use `context.Context` for long-running work and shell commands.
- Protect shared state with `sync.Mutex` or `sync.RWMutex`.
- Keep platform-specific IO behind build tags.
- Do not add external Go dependencies.

Frontend:

- Use native ES modules and relative imports.
- Escape dynamic HTML with helpers from `internal/server/static/js/util.js`.
- Use existing CSS tokens from `css/tokens.css`; avoid hardcoded colors.
- Keep UI screens under `internal/server/static/js/views`.
- Do not add React, Vue, Svelte, Tailwind, Vite, Webpack, or CDN assets.
- After frontend changes, verify in a browser or Playwright when practical.

Persistence:

- Preserve existing JSON field names and support old records where possible.
- Do not rewrite append-only audit logs.
- Do not regenerate signing keys except on first startup when missing.

## Build and Deployment

Docker build:

```bash
docker build -t usb-wiper:latest .
```

Typical runtime:

```bash
docker run --rm -it --privileged \
  -v /dev:/dev \
  -v /sys:/sys:ro \
  -v "$PWD/data:/data" \
  -p 8181:8181 \
  usb-wiper:latest
```

CI runs vet, race tests with coverage, a static Linux binary build, and a
Docker build. Keep docs aligned with `.github/workflows/ci.yml` and `Makefile`.

## Pull Request Guidelines

- Prefer conventional commits: `feat:`, `fix:`, `docs:`, `test:`, `chore:`,
  `ci:`.
- Run `make fmt`, `make vet`, and the relevant `make test*` target before
  handoff (all inside containers).
- Include focused tests for new stores, queue behavior, identity behavior, or
  API validation.
- Keep unrelated dirty worktree changes intact. Do not revert user changes.

## Debugging Tips

| Symptom | Likely cause |
| --- | --- |
| Device does not appear | Not USB according to `/sys`, or detection skipped it |
| USB SSD blocked as non-removable | UASP enclosure reports `removable=0`; use `UNSAFE_ALLOW_ALL_USB=1` only when appropriate |
| Old data appears after swapping drives in a USB enclosure | Code is using adapter/path identity instead of trusted physical disk identity |
| Auto Wipe does nothing | Feature is disabled, identity is low confidence, serial is empty, or device was already seen |
| Auto Wipe enabled but no job queued | Correct if the drive was already connected when enabling |
| SMART is empty | USB bridge may need a different `smartctl -d` mode; current code tries known bridges |
| SSE stops | Check `WriteTimeout`; main server must keep `WriteTimeout: 0` for SSE |
| Data not persisted | `DATA_DIR` is not mounted or writable |
| Certificate unavailable | Job must be completed or failed before certificate download |

## Known Gaps

- Authentication is not implemented; deploy behind an authenticating reverse
  proxy if the appliance is exposed beyond a trusted network.
- Wipe and health history grow unbounded; no retention trimming is performed.
- Hardware secure erase needs real-device validation; CI cannot cover it.
