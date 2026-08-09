---
layout: default
title: Configuration
description: All TOML configuration keys for easywall-core and easywall-web explained.
---

# Configuration

Two TOML files in `/etc/easywall/`, one per process, read at startup. A bad value is
a clean exit with a message — never a silent fallback.

| | Owned by | Holds |
|---|---|---|
| `easywall.toml` | core, root | firewall options, acceptance window, IPv6, Docker |
| `web.toml` | web, `root:easywall` `0640` | bind address, TLS, session secret, credentials |

Both binaries can write a commented default:

```bash
sudo easywall-core --write-config /etc/easywall/easywall.toml
sudo easywall-web  --write-config /etc/easywall/web.toml
```

---

## easywall-core (`/etc/easywall/easywall.toml`)

### Top-Level Keys

| Key | Type | Default | Description |
|---|---|---|---|
| `socket_path` | string | `/run/easywall/core.sock` | Unix socket path — must be accessible to the `easywall` group |
| `data_dir` | string | `/var/lib/easywall` | Directory for `rules.json` and version cache |
| `log_dir` | string | `/var/log/easywall` | Directory for audit log and rule snapshots |

### `[acceptance]`

The two-step activation safety mechanism. When a ruleset is applied, the core waits up to `duration` seconds for an explicit acceptance signal. If no signal arrives, the previous ruleset is automatically restored.

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Enable two-step activation safety |
| `duration` | int | `120` | Seconds before auto-rollback if not confirmed — 10 to 3600 |

Set `duration` to a value long enough for you to verify connectivity from a second terminal after applying rules.

The range is enforced, not merely suggested. Below ten seconds the window closes before
the confirmation page can be read, so every apply rolls back and the firewall can no
longer be changed through the interface. A value outside the range in an existing file
is brought to the nearest permitted one, with a warning, rather than keeping the daemon
from starting; a value set through the interface is rejected outright.

### `[ipv6]`

| Key | Type | Default | Description |
|---|---|---|---|
| `mode` | string | `"filter"` | What happens to IPv6: `filter` puts it through every rule, `passthrough` accepts it before any rule, `block` drops it except loopback |
| `icmp_allow_router_advertisement` | bool | `true` | Allow ICMPv6 type 134 — required for SLAAC address autoconfiguration |
| `icmp_allow_neighbor_advertisement` | bool | `true` | Allow ICMPv6 types 135/136 — required for Neighbor Discovery Protocol |

Both ICMPv6 keys apply only under `mode = "filter"`. Under `passthrough` the traffic
is already accepted and under `block` already gone.

> **`enabled` is obsolete.** It was documented as "off means IPv6 traffic is not
> filtered at all" and did the opposite: the table is `inet`, so every rule and the
> drop policy still applied to IPv6 and only the ICMPv6 exemptions were removed —
> IPv6 came out filtered *and* non-functional. A config still carrying the key loads,
> and both old values become `mode = "filter"`.

Disable `enabled` only on servers with no IPv6 addressing. Disabling individual ICMPv6 RA/NA types will break IPv6 connectivity.

### `[docker]`

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Auto-detect Docker bridge interfaces and whitelist them |
| `allow_bridge_networks` | bool | `true` | Whitelist auto-detected bridge network CIDRs |
| `custom_networks` | list | `[]` | Additional CIDRs to whitelist unconditionally (processed when `enabled = true`) |

See [Docker Coexistence]({{ '/features/docker/' | relative_url }}) for the full setup guide.

### `[firewall]` — Protection Modules

Each module has a matching `_log` boolean and one or more numeric threshold keys. The table shows the primary on/off toggle; see [Firewall Filters]({{ '/features/filters/' | relative_url }}) for details on each module.

| Key | Type | Default | Description |
|---|---|---|---|
| `ssh_brute_force` | bool | `true` | Rate-limit new connections to SSH-tagged ports |
| `ssh_brute_force_log` | bool | `false` | Log rate-limited SSH attempts |
| `ssh_brute_force_connection_limit` | int | `5` | New SSH connections per minute from one source address |
| `ssh_brute_force_log_limit` | int | `60` | Log entries per minute |
| `icmp_flood` | bool | `true` | Rate-limit echo requests per source address — ICMP type 8 and ICMPv6 type 128 |
| `icmp_flood_log` | bool | `false` | Log rate-limited ICMP |
| `icmp_flood_connection_limit` | int | `10` | Echo requests per second from one source address |
| `icmp_flood_log_limit` | int | `60` | Log entries per minute |
| `syn_flood` | bool | `true` | Rate-limit new TCP connections per source address |
| `syn_flood_log` | bool | `false` | Log rate-limited SYN packets |
| `syn_flood_limit` | int | `100` | New TCP connections per second from one source address |
| `port_scan` | bool | `true` | Drop TCP packets with suspicious flag combos |
| `port_scan_log` | bool | `false` | Log dropped port scan packets |
| `drop_invalid_packets` | bool | `true` | Drop packets in INVALID conntrack state |
| `drop_invalid_packets_log` | bool | `false` | Log dropped invalid packets |
| `drop_fragments` | bool | `false` | Drop IP-fragmented packets |
| `drop_fragments_log` | bool | `false` | Log dropped fragments |
| `bogon_filter` | bool | `false` | Drop impossible IPv4 source addresses arriving on a non-loopback interface |
| `bogon_filter_log` | bool | `false` | Log bogon-filtered packets |
| `connection_limit_per_ip` | bool | `false` | Limit simultaneous connections per source IP |
| `connection_limit_max` | int | `100` | Max simultaneous connections per source IP |
| `tcp_rst_flood` | bool | `false` | Rate-limit inbound TCP RST packets per source address |
| `tcp_rst_flood_log` | bool | `false` | Log rate-limited RST packets |
| `tcp_rst_flood_limit` | int | `100` | RST packets per second from one source address |
| `drop_broadcast` | bool | `false` | Drop broadcast-destination packets |
| `drop_multicast` | bool | `false` | Drop multicast-destination packets |
| `drop_anycast` | bool | `false` | Drop anycast packets |
| `log_blocked_connections` | bool | `false` | Add rate-limited log rule before the final DROP |
| `log_blocked_connections_limit` | int | `60` | Log entries per minute for the final DROP log |
| `log_blacklist_connections` | bool | `false` | Log packets matched by the blacklist |
| `log_blacklist_connections_limit` | int | `60` | Log entries per minute for blacklist drops |

---

## easywall-web (`/etc/easywall/web.toml`)

### Top-Level Keys

| Key | Type | Description |
|---|---|---|
| `bind_addr` | string | Listen address and port — e.g. `"0.0.0.0:12227"` or `"127.0.0.1:12227"` |
| `socket_path` | string | Path to the core Unix socket — must match `easywall.toml` |
| `ssl_dir` | string | Directory where the auto-generated TLS cert/key are stored |
| `data_dir` | string | Directory for the version cache file — defaults to `/var/lib/easywall` |
| `language` | string | Fallback UI locale — `"en"` (English) or `"de"` (German). Only used when the browser asks for a language easywall does not have and no choice has been made in the interface |
| `session_key` | string | 32-byte hex secret that signs the session cookie. Generated on first start if missing or still the shipped placeholder |
| `username` | string | Login username — set via the first-run wizard |
| `password` | string | Argon2id hash — set via the first-run wizard, do not edit by hand |
| `update_check` | bool | Ask github.com once a day whether a newer release exists — `true` by default. The only outbound request easywall makes; see below |
| `demo_mode` | bool | Run against an in-memory mock instead of the core. For the public demo only — never on a host you are protecting |

### How the interface picks a language

Highest priority first:

1. **An explicit choice in the interface.** The switch in the sidebar footer — and
   on the login page, so an operator who cannot read it can still get in — stores
   `easywall_lang` for a year. This outranks everything below: the browser header
   describes the machine, and this describes the person using it.
2. **The browser's `Accept-Language` header.**
3. **`language` in the config**, the setting above.
4. **English.**

The languages on offer are whatever `locales/*.json` contains, and each file names
itself through its own `language_name` key — so `Deutsch` reads as `Deutsch`
whatever language the interface is currently in. Adding a locale file is all it
takes for it to appear in the switch; see [Adding a Language](contributing.md).

```bash
openssl rand -hex 32     # session_key
```

**Keep `session_key` private.** Anyone holding it can forge a valid session cookie —
no password required. easywall generates one on first start if the key is missing, too
short, or still the placeholder the sample config ships with, and writes it back to
`web.toml`.

> **There is no `csrf_key`.** CSRF protection is Go 1.25's
> `net/http.CrossOriginProtection`, which checks `Origin` and `Sec-Fetch-Site` rather
> than issuing tokens. A `csrf_key` left over from an older config is read by nothing.

### `[tls]`

Leave both keys empty to use an auto-generated self-signed certificate in `ssl_dir`.

| Key | Description |
|---|---|
| `cert` | Absolute path to a custom TLS certificate PEM file (e.g. Let's Encrypt fullchain) |
| `key` | Absolute path to the matching private key PEM file |

The auto-generated certificate is valid for a year and is replaced once it comes within
30 days of expiry — checked at startup and twice a day while the service runs, so a
server that stays up past its own certificate keeps working.

A custom certificate is **never** overwritten. It is re-read when the file changes, so
an ACME client renewing it in place takes effect on the next connection without a
restart.

Set both `cert` and `key` or neither. Setting one alone is refused at startup: easywall
would otherwise pair your file with the other half of its own generated pair, and TLS
fails with a key-mismatch error naming a certificate you never configured.

## The update check

The dashboard asks `api.github.com` once a day whether there is a newer release, and
shows a banner if so. It is the only outbound request easywall makes, and it is never
in the way of a page: the answer comes from a cache on disk, and a stale cache is
refreshed in the background. On a host with no route out, the failure is remembered for
an hour rather than retried on every load.

Set `update_check = false` to switch it off entirely. Nothing else changes; the version
easywall is running is shown either way.

---

## Editor autocompletion

Both files ship a JSON Schema. Point [Taplo](https://taplo.tamasfe.dev/) at them for
inline validation in VS Code, Neovim and anything else speaking LSP:

```toml
# taplo.toml (project root)
[[rule]]
include = ["config/easywall.toml"]
url = "https://jp1337.github.io/easywall/schemas/easywall.schema.json"

[[rule]]
include = ["config/web.toml"]
url = "https://jp1337.github.io/easywall/schemas/web.schema.json"
```

Direct schema links:

- [easywall.schema.json]({{ '/schemas/easywall.schema.json' | relative_url }})
- [web.schema.json]({{ '/schemas/web.schema.json' | relative_url }})
