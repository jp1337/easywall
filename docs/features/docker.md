---
layout: default
title: Docker Coexistence
description: How easywall and Docker coexist — own nftables table, bridge auto-detection, three setup options.
---

# Docker Coexistence

## The Problem (v1)

In easywall v1, Docker and easywall frequently conflicted. Docker manages
its own network chains (`DOCKER`, `DOCKER-USER`, `DOCKER-ISOLATION`) in the
iptables `filter` table. When easywall flushed all iptables rules, it wiped
Docker's chains and broke container networking.

## The Solution (v2)

easywall v2 uses its **own nftables table**: `table inet easywall`.

Docker's iptables/nftables chains live in the `filter` table. easywall
never touches the `filter` table — it only manages `inet easywall`. The two
systems coexist without interfering.

Additionally, when Docker mode is enabled, easywall auto-detects Docker bridge
networks and adds `ACCEPT` rules for them in `inet easywall`.

## Configuration

Enable Docker mode in `/etc/easywall/easywall.toml`:

```toml
[docker]
enabled               = true   # auto-detect Docker bridge interfaces
allow_bridge_networks = true   # whitelist detected bridge networks
custom_networks       = []     # add any extra networks manually
```

Restart the core after changing this:

```bash
systemctl restart easywall-core
```

## How Auto-Detection Works

easywall calls `net.Interfaces()` at startup and looks for interfaces whose
names start with `docker` or `br-`. It reads the CIDR of each such interface
and generates `ACCEPT` rules for traffic to/from those networks.

## Setup Options

### Option 1: Docker Mode enabled (recommended)

easywall and Docker coexist. Container traffic is automatically whitelisted.

```toml
[docker]
enabled = true
```

### Option 2: Docker with `--iptables=false`

Disable Docker's own firewall rules entirely. easywall manages everything.
Add to `/etc/docker/daemon.json`:

```json
{"iptables": false}
```

Restart Docker, then manage all container port exposure via easywall rules.

<div class="callout callout-warning">
  <svg class="callout-icon" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd"/></svg>
  <div class="callout-content">
    <strong>Warning</strong>
    <p>With <code>iptables: false</code>, published ports (<code>-p 80:80</code>) are no longer automatically exposed. You must manually create TCP/UDP rules in easywall for every container port you want to reach from outside.</p>
  </div>
</div>

### Option 3: easywall only for host, Docker manages its own networking

Leave Docker mode disabled (`enabled = false`) and don't use Docker's published
ports from external networks. Containers can still reach the internet via Docker's
NAT masquerade.
