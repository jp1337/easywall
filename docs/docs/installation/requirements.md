# Requirements

## System Requirements

| Requirement | Minimum | Notes |
|---|---|---|
| **Kernel** | 3.13+ | nftables introduced in 3.13 |
| **nftables** | any | `apt install nftables` |
| **init system** | systemd | required for service management |
| **Architecture** | amd64 / arm64 | pre-built binaries available |
| **RAM** | ~32 MB | combined for both processes |

## Supported Distributions

easywall is tested on:

| Distribution | Status |
|---|---|
| Debian 12 (Bookworm) | ✅ Fully supported |
| Debian 11 (Bullseye) | ✅ Fully supported |
| Ubuntu 24.04 LTS | ✅ Fully supported |
| Ubuntu 22.04 LTS | ✅ Fully supported |
| Raspbian (Debian 12) | ✅ arm64 binary |

Other systemd-based distributions with nftables support should work but are
not tested in CI.

## Ports

| Port | Process | Protocol | Purpose |
|---|---|---|---|
| 12227 | easywall-web | HTTPS (TLS 1.2+) | Web interface |

## Not Required

- Python / pip
- Node.js / npm
- Any database (SQLite, PostgreSQL, etc.)
- Docker (unless using the Docker deployment option)
