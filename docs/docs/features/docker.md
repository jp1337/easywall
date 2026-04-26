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

!!! warning
    With `iptables: false`, published ports (`-p 80:80`) are no longer
    automatically exposed. You must manually create TCP/UDP rules in easywall
    for every container port you want to reach from outside.

### Option 3: easywall only for host, Docker manages its own networking

Leave Docker mode disabled (`enabled = false`) and don't use Docker's published
ports from external networks. Containers can still reach the internet via Docker's
NAT masquerade.
