---
layout: default
title: Configuration
description: All TOML configuration keys for easywall-core and easywall-web explained.
---

# Configuration

Two TOML files in `/etc/easywall/`, one per process, read at startup.

A value that cannot be interpreted stops the daemon with a message naming the key —
an unknown `ipv6.mode`, a missing path. A value that is merely out of range is brought
into range and **said out loud** in the log, because a firewall daemon that refuses to
start is a worse outcome than one running a documented default: `acceptance.duration`
is clamped to 10–3600, and a rate limit of zero on an enabled module becomes that
module's default.

The same value arriving through the interface is refused instead, with the key named.
Nothing is substituted quietly in either direction — that used to be five rate limits,
and the file and the running firewall could disagree with nothing to say so.

| | Owner and mode | Holds |
|---|---|---|
| `easywall.toml` | `root:root` `0600` | firewall options, acceptance window, IPv6, Docker, routing |
| `web.toml` | `easywall:easywall` `0600` | bind address, TLS, session secret, credentials |

The split is the point: the web process rewrites its own file — the wizard and the
password page write into it — and must not be able to touch the one the root daemon
reads.

The package and the container install both, already filled in — from
`*.toml.template`, created once and never touched again, because easywall edits
both files itself and a file a program rewrites must not be managed by the
package manager. An upgrade replaces the templates and leaves your two files
alone. Either binary can also write a commented default — it carries one, so this
works on a host that has nothing but the binary:

```bash
sudo easywall-core --write-config /etc/easywall/easywall.toml
sudo easywall-web  --write-config /etc/easywall/web.toml
```

**It never overwrites.** Both paths hold a working firewall's settings once
easywall is running, and `web.toml` also holds the session key and the password
hash; pointed at a file that exists, the command says so and changes nothing.

## The command line

Three flags, and nothing else.

| | |
|---|---|
| `-config <path>` | which file to read. Defaults to `/etc/easywall/easywall.toml` and `/etc/easywall/web.toml` |
| `-write-config <path>` | write the commented default to that path, `0600`, and exit. Refuses if the file exists, and does not create the directory |
| `-version` | print the version and exit — what the binary was actually built as |

```bash
easywall-core --version     # easywall-core {{ site.version }}
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

Turning either ICMPv6 key off breaks IPv6 on most networks — SLAAC and neighbour
discovery are how an address is obtained and kept reachable. They exist for hosts
with static addressing that genuinely need neither.

### `[docker]`

| Key | Type | Default | Description |
|---|---|---|---|
| `enabled` | bool | `false` | Auto-detect Docker bridge interfaces and whitelist them |
| `allow_bridge_networks` | bool | `true` | Whitelist auto-detected bridge network CIDRs |
| `custom_networks` | list | `[]` | Additional CIDRs to whitelist unconditionally (processed when `enabled = true`) |

See [Docker Coexistence]({{ '/features/docker/' | relative_url }}) for the full setup guide.

### `[routing]`

Traffic this host passes on rather than receives — between two interfaces, out of a
container, into a published container port.

| Key | Type | Default | Description |
|---|---|---|---|
| `mode` | string | `"closed"` | `closed`, `networks` or `open` — see below |
| `networks` | list | `[]` | CIDRs that may be routed, in either direction. Read only under `mode = "networks"` |

| `mode` | What crosses the `forward` chain |
|---|---|
| `closed` | Nothing, beyond what `[docker]` allows. Correct for a plain server |
| `networks` | Also anything with a source *or* a destination in `networks` |
| `open` | Everything. easywall filters only what arrives for this host |

> **A closed `forward` chain is not the same as an unfiltered one.** A base chain
> whose rules give no verdict falls through to its policy, so an empty chain with
> `policy drop` destroys every routed packet — including ones another table's
> forward chain has already accepted. Until 2.5.0 that was the only behaviour and
> nothing said so, which meant every Docker container lost its network the moment
> easywall applied. Docker's networks now cross regardless of `mode`; anything else
> that routes needs this key.

### `[firewall]` — Protection Modules

One row per module: the switch that turns it on, what it may be tuned with, and
its own logging switch. Every threshold is **per source address**. All booleans
default to what the "on" column says; the numbers are the defaults shown.

What each module actually drops is on
[Firewall Filters]({{ '/features/filters/' | relative_url }}); this is the key
reference.

| Module | Switch — default | Tuning | Logging |
|---|---|---|---|
| SSH brute-force | `ssh_brute_force` — **on** | `ssh_brute_force_connection_limit` `5`/min | `ssh_brute_force_log`, `ssh_brute_force_log_limit` `60`/min |
| ICMP flood | `icmp_flood` — **on** | `icmp_flood_connection_limit` `10`/s | `icmp_flood_log`, `icmp_flood_log_limit` `60`/min |
| SYN flood | `syn_flood` — **on** | `syn_flood_limit` `100`/s | `syn_flood_log` |
| Port scan | `port_scan` — **on** | — | `port_scan_log` |
| Invalid packets | `drop_invalid_packets` — **on** | — | `drop_invalid_packets_log` |
| Fragments (IPv4) | `drop_fragments` — off | — | `drop_fragments_log` |
| Bogon filter (IPv4) | `bogon_filter` — off | — | `bogon_filter_log` |
| Connection limit | `connection_limit_per_ip` — off | `connection_limit_max` `100` | — |
| TCP RST flood | `tcp_rst_flood` — off | `tcp_rst_flood_limit` `100`/s | `tcp_rst_flood_log` |
| Broadcast | `drop_broadcast` — off | — | — |
| Multicast | `drop_multicast` — off | — | — |
| Anycast | `drop_anycast` — off | — | — |

Every number above has a permitted range, and it is the daemon that holds it —
the `max` on the options page and the `maximum` in the JSON Schema are hints to a
browser and an editor, and neither reaches a `curl` or a hand-edited file:

| Key | Range | Default |
|---|---|---|
| `ssh_brute_force_connection_limit` | 1–100 | 5 |
| `ssh_brute_force_log_limit` | 1–10000 | 60 |
| `icmp_flood_connection_limit` | 1–1000 | 10 |
| `icmp_flood_log_limit` | 1–10000 | 60 |
| `syn_flood_limit` | 1–10000 | 100 |
| `tcp_rst_flood_limit` | 1–10000 | 100 |
| `connection_limit_max` | 1–100000 | 100 |
| `log_blocked_connections_limit` | 1–10000 | 60 |
| `log_blacklist_connections_limit` | 1–10000 | 60 |

Out of range in the file is clamped and logged; out of range from the interface is
refused with the key named — the same split as `acceptance.duration`.

> **There was no upper bound at all until now, and these numbers reach 32-bit
> fields.** So too large did not fail, it wrapped. Measured against a kernel:
> `connection_limit_max = 5000000000` arrived as `ct count over 705032704`, and
> `4294967296` arrived as `ct count over 0` — a rule matching every connection
> from every source and dropping it. One number, entered on a page whose product
> promises it cannot lock you out, with nothing logged.

Two logging switches belong to no module and are set here as well:

| | Logs | Rate |
|---|---|---|
| `log_blocked_connections` | everything the final policy drops | `log_blocked_connections_limit` `60`/min |
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
| `data_dir` | string | Directory for the version cache and the installation identifier — defaults to `/var/lib/easywall` |
| `language` | string | Fallback UI locale — `"en"` (English) or `"de"` (German). Only used when the browser asks for a language easywall does not have and no choice has been made in the interface |
| `session_key` | string | Hex secret that signs the session cookie — `openssl rand -hex 32`, which is 64 characters. Optional: one is generated on first start and written back here if the key is missing, shorter than 32 characters, or still the shipped placeholder |
| `username` | string | Login username — set via the first-run wizard |
| `password` | string | Argon2id hash — set via the first-run wizard, do not edit by hand |
| `update_check` | bool | Ask github.com once a day whether a newer release exists — `true` by default. One of two possible outbound requests; see below |
| `telemetry` | bool | Whether this installation may be counted — off unless switched on, and asked during the first run. See below |
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
takes for it to appear in the switch; see
[Adding a language]({{ '/contributing/' | relative_url }}#adding-a-language).

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

## Every request that leaves the host

Two, and this is the whole list.

| | Update check | Counting installations |
|---|---|---|
| Key | `update_check` | `telemetry` |
| Default | **on** | **off** until you switch it on |
| Destination | `api.github.com` | `telemetry.wdkro.de` |
| How often | once a day | once a day |
| Carries | nothing about you — a plain GET for the newest release | a random identifier and the version, in full below |
| Switched off by | `update_check = false` | `telemetry = false`, or **System** in the interface |

Neither delays a page. The update check is served from a cache on disk and
refreshed in the background, and a failure is remembered for an hour so a host with
no route out is not retrying on every load. The count runs in the background and
gives up after ten seconds.

### The update check

A banner appears when a newer release exists. Switching it off changes nothing
else — the version easywall is running is shown either way.

### Counting installations

The first-run wizard asks rather than assumes. A critical bug matters differently
at ten installations than at ten thousand, and the count is the only way to know
which this is — or to say a fix has reached most of them.

What it sends, in full — once a day, nothing else, ever:

```
GET https://telemetry.wdkro.de/v1/count?id=<32 hex characters>&v=<version>
```

| | |
|---|---|
| **Not** sent | the hostname, any address, any rule, any count of what you have configured |
| The identifier | 16 random bytes generated on your machine, in `<data_dir>/telemetry.json`. Delete the file and the next report is a new installation as far as anyone can tell |
| Why random | a value derived from the hostname or machine-id can be reproduced by anyone who knows the host, which turns a count into a lookup |
| At the far end | one line — timestamp, identifier, version — and a 204. **Your address is not recorded**: it rate-limits the endpoint and never reaches disk. Lines are kept 35 days, then only the rolled-up number |

Turning it off does not need the core process to be running — consent that can only
be withdrawn while another daemon is reachable would not be consent.

> The number is a lower bound and cannot be made tamper-proof: the endpoint is open, so
> anyone can invent identifiers. It is good enough to tell ten installations from ten
> thousand, and to see whether a fix has spread. It is not good for anything else, and
> is not claimed to be.

---

## Editor autocompletion

Both files ship a JSON Schema. Point [Taplo](https://taplo.tamasfe.dev/) at them for
inline validation in VS Code, Neovim and anything else speaking LSP:

```toml
# taplo.toml (project root)
[[rule]]
include = ["config/easywall.toml"]
url = "https://easywall-project.org/schemas/easywall.schema.json"

[[rule]]
include = ["config/web.toml"]
url = "https://easywall-project.org/schemas/web.schema.json"
```

Direct schema links:

- [easywall.schema.json]({{ '/schemas/easywall.schema.json' | relative_url }})
- [web.schema.json]({{ '/schemas/web.schema.json' | relative_url }})
