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
