---
layout: default
title: Configuration
description: All TOML configuration keys for easywall-core and easywall-web explained.
---

# Configuration

easywall uses two TOML configuration files — one for each process.

## easywall-core (`/etc/easywall/easywall.toml`)

Controls the firewall daemon and all protection modules.

### `[firewall]`

| Key | Type | Default | Description |
|---|---|---|---|
| `ssh_brute_force` | bool | `true` | Rate-limit new SSH connections per source IP |
| `icmp_flood` | bool | `true` | Rate-limit ICMP echo requests |
| `syn_flood` | bool | `true` | Rate-limit new TCP SYN packets |
| `port_scan` | bool | `true` | Drop packets with suspicious TCP flag combos (NULL, FIN, XMAS) |
| `drop_invalid_packets` | bool | `true` | Drop packets in INVALID conntrack state |
| `drop_fragments` | bool | `false` | Drop IP fragments |
| `bogon_filter` | bool | `false` | Drop RFC-1918 traffic arriving from external interfaces |
| `connection_limit_per_ip` | bool | `false` | Limit simultaneous connections per source IP |
| `tcp_rst_flood` | bool | `false` | Rate-limit TCP RST packets |
| `drop_broadcast` | bool | `true` | Drop broadcast packets |
| `drop_multicast` | bool | `false` | Drop multicast packets |
| `log_blocked_connections` | bool | `false` | Log traffic before the final DROP rule |

Each `_log` variant (e.g. `ssh_brute_force_log`) toggles logging for that module.
Numeric `_limit` variants control rate thresholds (packets per second or max connections).

### `[acceptance]`

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Enable two-step activation safety |
| `duration` | int | `30` | Seconds before auto-rollback if not confirmed |

### `[ipv6]`

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `true` | Enable IPv6 rules |
| `icmp_allow_router_advertisement` | bool | `true` | Allow ICMPv6 RA (required for SLAAC) |
| `icmp_allow_neighbor_advertisement` | bool | `true` | Allow ICMPv6 NA (required for NDP) |

### `[docker]`

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Auto-detect Docker bridge interfaces |
| `allow_bridge_networks` | bool | `true` | Whitelist detected bridge networks |
| `custom_networks` | list | `[]` | Additional CIDRs to whitelist |

---

## easywall-web (`/etc/easywall/web.toml`)

Controls the web frontend.

| Key | Type | Description |
|---|---|---|
| `bind_addr` | string | Listen address, e.g. `"0.0.0.0:12227"` |
| `socket_path` | string | Path to core Unix socket |
| `ssl_dir` | string | Directory for auto-generated TLS certificates |
| `language` | string | Default locale (`"en"` or `"de"`) |
| `session_key` | string | HMAC secret for session cookies — keep private |
| `username` | string | Login username (set via first-run wizard) |
| `password` | string | Argon2id password hash (set via first-run wizard) |

### `[tls]`

Leave both fields empty to use the auto-generated self-signed certificate.

| Key | Description |
|---|---|
| `cert` | Path to custom TLS certificate (e.g. Let's Encrypt) |
| `key` | Path to corresponding private key |

---

## JSON Schema

TOML configs are validated by JSON Schema — copy `taplo.toml` to your project root and your editor will show inline validation and autocomplete.

- [easywall.schema.json]({{ '/schemas/easywall.schema.json' | relative_url }})
- [web.schema.json]({{ '/schemas/web.schema.json' | relative_url }})
