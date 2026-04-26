---
layout: default
title: Docker Installation
description: Run easywall with Docker Compose — network_mode host, NET_ADMIN capability.
---

# Docker Installation

## Prerequisites

- Docker Engine 24+ and Docker Compose v2
- Linux host with nftables support
- `NET_ADMIN` capability (available for privileged containers on any kernel)

## Quick Start

```bash
git clone https://github.com/jp1337/easywall.git
cd easywall
docker compose up -d
```

Open `https://localhost:12227` and complete the first-run wizard.

## Configuration

All persistent data lives in Docker volumes:

| Volume | Path in container | Purpose |
|---|---|---|
| `easywall_data` | `/var/lib/easywall` | Rules JSON, version cache |
| `easywall_logs` | `/var/log/easywall` | Audit log |
| `./config` | `/etc/easywall` | Config files (bind-mounted) |

The `config/` directory is bind-mounted so you can edit `easywall.toml` and
`web.toml` directly. Restart the container after changes:

```bash
docker compose restart
```

## Why `network_mode: host`?

nftables operates on the host kernel's network namespace. Without host networking,
rules applied inside the container would only affect container network traffic,
not the host. The `NET_ADMIN` capability allows the container process to issue
netlink calls that modify host-level nftables tables.

## Using a Custom TLS Certificate

Mount your certificate files and update `config/web.toml`:

```yaml
# docker-compose.yml
volumes:
  - /etc/letsencrypt:/etc/letsencrypt:ro
  - ./config:/etc/easywall
```

```toml
# config/web.toml
[tls]
cert = "/etc/letsencrypt/live/example.com/fullchain.pem"
key  = "/etc/letsencrypt/live/example.com/privkey.pem"
```

## Updating

```bash
docker compose pull
docker compose up -d
```
