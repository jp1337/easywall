---
layout: default
title: Demo Mode
description: The whole interface against an in-memory mock — no root, no nftables, no core daemon.
---

# Demo Mode

`easywall-web` alone, backed by RAM instead of a privileged daemon. Every page works,
every save is recorded, every apply runs the acceptance state machine. Nothing reaches
a firewall.

{% include demo-callout.html %}

| | |
|---|---|
| Needs | one binary |
| Does **not** need | root, `CAP_NET_ADMIN`, nftables, `easywall-core` |
| State | in memory; gone on restart |

## Running your own

```bash
sudo mkdir -p /etc/easywall/ssl /var/lib/easywall

sudo tee /etc/easywall/web.toml > /dev/null <<'EOF'
bind_addr   = "0.0.0.0:12227"
ssl_dir     = "/etc/easywall/ssl"
data_dir    = "/var/lib/easywall"
session_key = "REPLACE_WITH_openssl_rand_hex_32"
demo_mode   = true
username    = "demo"
password    = ""      # empty: the first visitor sets it through the wizard
[tls]
cert = ""
key  = ""
EOF

sudo easywall-web -config /etc/easywall/web.toml
```

The startup log confirms it:

```
demo mode active — using in-memory mock instead of core socket
```

> **Custom-rule syntax cannot be checked.** There is no `nft` binary, so the page says
> live validation is not running rather than reporting a verdict it has no basis for.
> It used to answer "no errors" whatever was typed — a false green on the one page
> where being wrong locks you out. Address-list validation runs in the web process and
> works normally.

## What visitors see

- A neutral **Demo** chip in the topbar of every page: *nothing reaches a real
  firewall; state resets periodically*
- A notice on the login card pointing at the credentials

Neither weakens anything: demo mode does not disable authentication, CSRF, the CSP or
the rate limiter. With no privileged process running, the worst case is confined to
the unprivileged web process and its data directory.

## Resetting it on a schedule

Restarting the process wipes the state. A timer is enough:

```ini
# /etc/systemd/system/easywall-web-reset.timer
[Timer]
OnCalendar=hourly
Persistent=true
[Install]
WantedBy=timers.target
```

```ini
# /etc/systemd/system/easywall-web-reset.service
[Service]
Type=oneshot
ExecStart=/bin/systemctl restart easywall-web.service
```

## When it does not work

| Symptom | Cause |
|---|---|
| "Core daemon unreachable" | `demo_mode = true` is missing, or the binary predates it. Check the startup log |
| State vanished | By design — it is in memory. For persistence you want a real install |
| Login refused after a few tries | The rate limiter is real here too: 5 attempts per 10 minutes. Restarting the process resets it |
