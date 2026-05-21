# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] — 2026-05-21

### Added

- USB device detection via `/sys/block` enumeration
- Single-pass zero write wipe engine with verification
- SMART health information display (smartctl)
- Auto-format as FAT32 after wipe (optional)
- Minimal web UI with real-time progress via Server-Sent Events
- 9-step safety verification before any destructive operation
- Docker deployment (dev + production)
- `UNSAFE_ALLOW_ALL_USB` env var to bypass removable check for UASP USB SSD enclosures
- GitHub Actions CI/CD pipeline (test, build, release to Docker Hub)
- Multi-arch Docker images (linux/amd64, linux/arm64)

[0.1.0]: https://github.com/michealchoudhary/usb-wiper/releases/tag/v0.1.0
