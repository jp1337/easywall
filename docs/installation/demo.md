---
layout: default
title: Demo Mode
description: Run easywall-web standalone with an in-memory mock — no nftables, no root, no privileged daemon. Ideal for public demos and UI testing.
---

# Demo Mode

<div class="callout callout-success">
  <svg class="callout-icon" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.857-9.809a.75.75 0 00-1.214-.882l-3.483 4.79-1.88-1.88a.75.75 0 10-1.06 1.061l2.5 2.5a.75.75 0 001.137-.089l4-5.5z" clip-rule="evenodd"/></svg>
  <div class="callout-content">
    <strong>Try the live demo right now</strong>
    <p><strong>URL:</strong> <a href="https://easywall.wdkro.de" target="_blank" rel="noopener">https://easywall.wdkro.de</a> &nbsp;·&nbsp; <strong>Username:</strong> <code>demo</code> &nbsp;·&nbsp; <strong>Password:</strong> <code>demo</code></p>
    <p>The instance auto-resets every 6 hours so visitor changes don't pile up. It always runs the latest <code>main</code> build because the publishing pipeline re-deploys it on every successful merge — read on for how that works.</p>
  </div>
</div>

Demo mode runs `easywall-web` against an **in-memory mock of the core daemon**. Every page is fully interactive, every save/apply/audit-log entry is recorded, but **nothing reaches a real firewall** — there is no Unix socket connection, no `easywall-core` process, no nftables changes, and no root privileges required.

This makes it trivial to:

- Host a **public demo** (like the one above) so prospective users can explore the UI before installing
- **Test UI changes** locally without a privileged environment
- Demonstrate features in a workshop or screencast without the risk of locking yourself out

## How it works

When `demo_mode = true` is set in `web.toml`, the bootstrap path swaps the real `CoreClient` for an in-memory state machine. From the perspective of every handler and every template, the API is identical — just backed by RAM instead of IPC.

The state machine seeds itself with realistic example data on startup (3 TCP ports, 1 UDP port, 1 whitelist entry, 1 blacklist entry, all firewall protections enabled, and a few audit log entries from "yesterday"), so the UI looks alive on first visit.

State **resets when the process restarts**. For a public demo, schedule a periodic `systemctl restart easywall-web` to wipe accumulated visitor changes.

## How the public demo at easywall.wdkro.de stays current

The reference public demo at [easywall.wdkro.de](https://easywall.wdkro.de) runs the
**`:edge` Docker tag** — `ghcr.io/jp1337/easywall:edge` (or the equivalent
on Docker Hub / Quay.io; see [Docker Installation]({{ '/installation/docker/' | relative_url }})
for all three mirror locations and the full tag scheme).

The lifecycle from `git push` to a refreshed demo:

1. A commit lands on `main`
2. `Test`, `Build`, `Security` workflows run — must pass
3. `Publish edge` workflow builds amd64 + arm64 natively, pushes the
   manifest as `:edge` and `:sha-<commit>` to all three registries
4. A `deploy-demo` job on a self-hosted intranet runner curls
   [Watchtower](https://containrrr.dev/watchtower/) on the demo host
5. Watchtower pulls the new digest, recreates the container, drops
   any visitor state from the previous instance

Typical wall-clock time from merge to live: a few minutes.

In addition to the per-commit refresh, the demo container is restarted
**every 6 hours** by a systemd timer to wipe state that accumulated
between deploys (visitor-typed rules, audit log entries, etc.). The
two mechanisms are independent — both push the demo back toward its
seed state, just at different cadences.

If you self-host a demo following this guide, point your image reference
at `:edge` if you want the same auto-rolling behaviour, or `:latest` if
you want only released versions.

## Running the demo

### Single-binary deployment

You only need `easywall-web` — `easywall-core` is not started. There are no privileges to drop, no `CAP_NET_ADMIN`, no `nftables` dependency.

```bash
# Build (or download the release binary)
make build

# Create a config directory
sudo mkdir -p /etc/easywall /var/lib/easywall /etc/easywall/ssl

# Write a minimal demo config
sudo tee /etc/easywall/web.toml > /dev/null <<'EOF'
bind_addr   = "0.0.0.0:12227"
ssl_dir     = "/etc/easywall/ssl"
data_dir    = "/var/lib/easywall"
session_key = "REPLACE_WITH_openssl_rand_hex_32"
language    = "en"
demo_mode   = true

# Set to whatever credentials you want to expose on the demo landing page
username = "demo"
password = ""        # leave empty on first run, the wizard will set it

[tls]
cert = ""
key  = ""
EOF

# Start it
sudo /usr/sbin/easywall-web -config /etc/easywall/web.toml
```

Visit `https://your-server:12227`, complete the first-run wizard (or pre-set credentials in the config), and explore. Every save lands in memory, every apply transitions the mock acceptance state machine, every page reflects the change.

### What's mocked

| Command | Behavior in demo |
|---------|------------------|
| `GetStatus`, `GetRules`, `GetOptions`, `GetSettings`, `GetSystem`, `GetLog` | Returns in-memory state |
| `SaveRules`, `SaveOptions`, `SaveSettings`, `SaveSystem` | Updates in-memory state, appends an audit log entry |
| `ApplyRules` | Promotes Staged → Current, starts an acceptance timer if enabled |
| `Accept` | Cancels the timer, transitions to Accepted |
| acceptance timeout | Auto-rolls back Current → Backup, sets state to RolledBack |
| `ValidateCustom` | Always returns "no errors" — there is no `nft` binary to call |
| `ExportRules` / `ImportRules` | Round-trips the in-memory rule set as JSON |

### What's missing

The demo cannot validate `nftables` syntax (no `nft` binary), so the Custom Rules page will accept any input. A small notice on the page makes this clear.

## Visual indicators

In demo mode the UI shows two persistent visual cues so visitors are not confused about what they're looking at:

- **Topbar banner** on every authenticated page: an amber strip reading *"Demo Mode — every page is fully interactive, but nothing is applied to a real firewall. State resets on server restart."*
- **Login screen notice**: a soft alert points to the demo landing page for credentials.

## Periodic reset (recommended)

For a publicly hosted demo, restart the web binary at a regular interval to wipe visitor state:

```ini
# /etc/systemd/system/easywall-web-reset.timer
[Unit]
Description=Reset easywall demo state hourly

[Timer]
OnCalendar=hourly
Persistent=true

[Install]
WantedBy=timers.target
```

```ini
# /etc/systemd/system/easywall-web-reset.service
[Unit]
Description=Restart easywall-web to wipe demo state
Requires=easywall-web.service

[Service]
Type=oneshot
ExecStart=/bin/systemctl restart easywall-web.service
```

```bash
sudo systemctl enable --now easywall-web-reset.timer
```

## Security notes

- Demo mode does **not** disable authentication — visitors must still log in. Set `username` and `password` in the config, or expose the first-run wizard and let the first visitor claim the credentials (then restart to reset).
- Demo mode does **not** weaken CSP, CSRF, or any other request-level protections.
- Because no privileged process runs, the worst-case impact of a vulnerability in demo mode is limited to the unprivileged `easywall-web` process and its data directory.

## Troubleshooting

**The dashboard says "Core daemon unreachable"**

Either you forgot to set `demo_mode = true`, or the binary you're running is too old. Check the startup log — the line `demo mode active — using in-memory mock instead of core socket` appears immediately after `easywall-web started` when demo mode is correctly configured.

**State doesn't persist across restarts**

That's intentional — demo mode is in-memory only. If you want persistence, you want a real production install with `easywall-core`, not the demo.

**Custom rules don't show validation errors**

Demo mode has no `nft` binary to call. Every input is accepted as syntactically valid. This is documented inline in the Custom Rules page.
