
## [0.5.5] - 2026-05-22


### Documentation

- Update screenshot




## [0.5.4] - 2026-05-22


### Fixed

- Trigger release workflow via workflow_dispatch instead of relying on tag push




## [0.5.3] - 2026-05-22


### Chore

- Add .pi-lens/ to .gitignore




## [0.5.2] - 2026-05-22


### Fixed

- Return Job copies from Get and List to prevent data race




## [0.5.1] - 2026-05-22


### Fixed

- Use RELEASE_PAT for auto-tag push to trigger release workflow




## [0.5.0] - 2026-05-22


### Added

- V0.5.0 — job queue, multi-scheme wipe, presets, certificates, and redesigned UI




## [0.4.0] - 2026-05-22


### Added

- Push-driven UI with wipe history persistence and random-chunk verification
- Concise log messages with milestone-based progress reporting
- Add non-destructive 'Test Wipe' button for read-only verification


### Chore

- Update test label for VerifyRandomChunks


### Fixed

- Allow CORS for all local network origins (10.x, 192.168.x)
- Require privileged mode for USB hotplug in docker-compose
- NVMe health detection and GitHub Actions checkout pin
- Remove RELEASE_PAT token from auto-tag checkout




## [0.3.0] - 2026-05-21


### Added

- Support wiping multiple USB devices simultaneously




## [0.1.2] - 2026-05-21


### Fixed

- Correct version prefix in auto-tag workflow




## [0.1.1] - 2026-05-21


### Fixed

- Replace adduser with useradd for debian:stable-slim




## [0.1.0] - 2026-05-21


### Added

- Switch to debian:stable-slim, add Renovate + git-cliff


### Chore

- Upgrade Go 1.22→1.26 and Alpine 3.19→3.23


### Ci

- Simplify to single Go version and single arch builds
- Add auto-tag workflow and git-cliff version bumping


### Documentation

- Add Docker Hub info, screenshot, and environment variables to README
- Update screenshot with actual rendered UI



# Changelog

All notable changes to this project will be documented in this file.
The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-05-21

### Added

- USB device detection via `/sys/block` enumeration
- Single-pass zero write wipe engine with verification
- SMART health information display (smartctl)
- Auto-format as FAT32 after wipe (optional)
- Minimal web UI with real-time progress via Server-Sent Events
- 9-step safety verification before any destructive operation
- `UNSAFE_ALLOW_ALL_USB` env var to bypass removable check for UASP USB SSD enclosures
- Docker Hub image at `michealchoudhary/usb-wiper`
- GitHub Actions CI/CD pipeline (test, build, release)
- Renovate bot integration for automated dependency updates
- Auto-versioning and changelog via git-cliff on merge to main
- Debian stable-slim base image with apt package manager

### Changed

- Go toolchain upgraded from 1.22 to 1.26
- Base image switched from Alpine 3.19 to Debian stable-slim
- Removed multi-arch Docker builds — single `linux/amd64`
- Safety check #6 (removable flag) is now skippable via `UNSAFE_ALLOW_ALL_USB=1`
- USB detection uses `filepath.EvalSymlinks` instead of raw `os.Readlink`
- Device listing now shows USB SSDs even when `removable=0`

### Fixed

- SSE streaming broken by middleware `responseWriter` wrapper not implementing `http.Flusher`
- `/proc/mounts` bind-mount failing on Docker Desktop (macOS)
- USB SSD enclosures not detected due to non-removable sysfs flag
- `ListUSBDevices` returning `nil` instead of empty slice on JSON serialization
