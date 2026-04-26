# easywall

**easywall** is a secure web interface for managing nftables firewall rules on Linux.
It is designed for system administrators who need a reliable, easy-to-use firewall
management tool without requiring deep nftables expertise.

## Why easywall in 2026?

Linux servers — and increasingly Linux desktops — remain high-value targets.
Many hosting providers (such as Strato) offer no upstream firewall, leaving
individual machines exposed. The Linux desktop market share is growing, developer
laptops run on Linux, and simple firewall tools are scarce. easywall fills this gap
for those who are not nftables experts.

## Architecture

easywall uses a **two-process design** for security:

```
[Browser]
    │ HTTPS (port 12227)
    ▼
[easywall-web]    ← unprivileged user: easywall
    │ Unix socket /run/easywall/core.sock (mode 660, group easywall)
    │ Typed JSON protocol
    ▼
[easywall-core]   ← root
    │ github.com/google/nftables (direct netlink — no subprocess)
    ▼
[nftables kernel]
```

The web process never touches the kernel directly. All firewall operations go
through the core daemon via a typed JSON protocol. This eliminates entire classes
of injection vulnerabilities.

## Key Features

| Feature | Description |
|---|---|
| **nftables backend** | Direct netlink API — no `nft` subprocess, no injection risk |
| **Two-step activation** | Rules auto-roll back if SSH connectivity is not confirmed |
| **Docker coexistence** | Own table `inet easywall` — never touches Docker's chains |
| **Argon2id auth** | Industry-standard password hashing |
| **CSRF protection** | gorilla/csrf on all POST requests |
| **i18n** | English and German, easily extensible |
| **Light/Dark mode** | OS preference detection + manual toggle |
| **Export/Import** | JSON rule backups |

## Quick Start (Debian / Ubuntu)

```bash
# Install the package
sudo dpkg -i easywall_2.0.0_amd64.deb
sudo apt-get install -f

# Open the web interface
xdg-open https://localhost:12227
```

The first-run wizard will ask you to set a username and password.

## Quick Start (Docker)

```bash
git clone https://github.com/jp1337/easywall.git
cd easywall
docker compose up -d
xdg-open https://localhost:12227
```

!!! warning "Network mode"
    Docker Compose uses `network_mode: host` and `NET_ADMIN` capability
    so the core can manage nftables on the host kernel. This is required
    for a host-level firewall.
