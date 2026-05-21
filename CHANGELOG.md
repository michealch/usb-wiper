# Changelog

## v0.1.0 — 2026-05-21

Initial release.

- USB device detection via `/sys/block` enumeration
- Single-pass zero write wipe engine
- SMART health information display (smartctl)
- Auto-format as FAT32 after wipe (optional)
- Minimal web UI with real-time progress via Server-Sent Events
- 9-step safety verification before any destructive operation
- Docker deployment (dev + production)
- No external Go dependencies (stdlib only)
- MIT License
