---
layout: default
title: Requirements
description: System requirements, supported distributions, kernel prerequisites, and upgrade notes for easywall.
---

# Requirements

## System Requirements

| Requirement | Minimum | Notes |
|---|---|---|
| **Kernel** | 3.13+ | nftables netlink API introduced in 3.13 |
| **nftables** | any | `apt install nftables` on Debian/Ubuntu |
| **init system** | systemd | required for service management and socket activation |
| **Architecture** | amd64 or arm64 | pre-built binaries available for both |
| **RAM** | ~32 MB | combined for both processes at idle |
| **Disk** | ~20 MB | binaries + assets + config |
| **Root/CAP_NET_ADMIN** | required | the core daemon must have `CAP_NET_ADMIN` to write nftables rules |

## Supported Distributions

easywall is tested on:

| Distribution | Architecture | Status |
|---|---|---|
| Debian 12 (Bookworm) | amd64, arm64 | ✅ Fully supported |
| Debian 11 (Bullseye) | amd64, arm64 | ✅ Fully supported |
| Ubuntu 24.04 LTS | amd64, arm64 | ✅ Fully supported |
| Ubuntu 22.04 LTS | amd64, arm64 | ✅ Fully supported |
| Raspbian (based on Debian 12) | arm64 | ✅ arm64 binary |

Other systemd-based distributions with nftables support (Arch, Fedora, openSUSE) should work but are not tested in CI.

## Network Ports

| Port | Process | Protocol | Purpose |
|---|---|---|---|
| 12227 | easywall-web | HTTPS (TLS 1.2+) | Web interface |

Only one inbound port is required. easywall-core communicates with easywall-web via a Unix socket — no additional TCP/UDP ports are opened between the two processes.

## nftables Prerequisites

The `nftables` package must be installed:

```bash
# Debian / Ubuntu
sudo apt-get install nftables

# Arch Linux
sudo pacman -S nftables

# Fedora / RHEL
sudo dnf install nftables
```

easywall uses the `inet easywall` table exclusively. It does not touch any pre-existing tables or chains. If you have existing nftables rules from another tool, they will not be affected.

<div class="callout callout-warning">
  <svg class="callout-icon" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd"/></svg>
  <div class="callout-content">
    <strong>iptables and nftables conflict</strong>
    <p>If your system uses both iptables-legacy and nftables, the rule sets may interfere with each other. Check with <code>update-alternatives --list iptables</code>. On modern Debian/Ubuntu the iptables binary already delegates to nftables via <code>iptables-nft</code>.</p>
  </div>
</div>

## Upgrading from easywall v1 (Python)

easywall v1 used iptables. v2 uses nftables. Before installing v2:

1. **Stop the v1 services** and disable them:
   ```bash
   sudo systemctl stop easywall easywall-web
   sudo systemctl disable easywall easywall-web
   ```

2. **Clear old iptables rules** (optional — they are independent of nftables):
   ```bash
   sudo iptables -F
   sudo iptables -X
   sudo ip6tables -F
   sudo ip6tables -X
   ```

3. **Remove the Python package** if installed via pip:
   ```bash
   pip uninstall easywall
   ```

4. Install easywall v2. Your old `rules.yml` (YAML) cannot be imported directly —
   use the **Export/Import** feature (JSON format) after setting up v2.

## Not Required

- Python, pip, or any Python runtime
- Node.js or npm — the stylesheet is compiled and shipped; the toolchain is only needed to rebuild it while developing
- Any database (SQLite, PostgreSQL, etc.)
- Docker (unless you use the Docker deployment option)
- Go toolchain on the target server — install the pre-built binary or `.deb` package
- **Outbound internet access.** The interface serves its own fonts, stylesheet and
  scripts, so it renders correctly on an air-gapped host. Only the optional update
  check contacts the network, and it fails quietly when it cannot.
