---
layout: default
title: Environment Variables
description: Every environment variable easywall reads, and why the list stops where it does.
---

# Environment Variables

Every variable easywall reads, the TOML key it names, and whether the interface
has a control for the same value.

## Which value wins

{% include themed-figure.html base="/assets/diagrams/config-precedence" ext="svg"
   alt="Stored value beats environment variable beats built-in default" %}

| Environment | Stored | In effect |
|---|---|---|
| — | — | the built-in default |
| set | — | the environment |
| set | differs | the stored value |
| set | identical | the stored value — which is the same thing |
| removed | present | the stored value |

**Stored** means the file's value differs from the built-in default — not that
the key is present in the file. `easywall-core -write-config` and
`easywall-web -write-config` emit every default, and that is exactly the file the
container image ships; if presence counted, every variable in this page would be
dead on every containerised installation.

A control that names a key you have also set through the environment says so on
the page, and offers **Reset to the environment value** when your stored value is
the one in effect.

## `easywall-core`

| Variable | `easywall.toml` key | Type | Control | Purpose |
|---|---|---|---|---|
| `EASYWALL_CORE_SOCKET_PATH` | `socket_path` | string | — | Unix socket path — must be accessible to the `easywall` group |
| `EASYWALL_CORE_DATA_DIR` | `data_dir` | string | — | Directory for `rules.json`, the last-apply state and the panic marker |
| `EASYWALL_CORE_LOG_DIR` | `log_dir` | string | — | Directory for the audit log and rule snapshots |

## `easywall-web`

| Variable | `web.toml` key | Type | Control | Purpose |
|---|---|---|---|---|
| `EASYWALL_WEB_BIND_ADDR` | `bind_addr` | string | — | Listen address and port — e.g. `0.0.0.0:12227` |
| `EASYWALL_WEB_SOCKET_PATH` | `socket_path` | string | — | Path to the core Unix socket — must match `easywall.toml` |
| `EASYWALL_WEB_SSL_DIR` | `ssl_dir` | string | — | Directory where the auto-generated TLS cert/key are stored |
| `EASYWALL_WEB_DATA_DIR` | `data_dir` | string | — | Directory for the version cache and the installation identifier |
| `EASYWALL_WEB_TLS_CERT` | `tls.cert` | string | — | Path to a custom TLS certificate PEM file |
| `EASYWALL_WEB_TLS_KEY` | `tls.key` | string | — | Path to the matching private key PEM file |
| `EASYWALL_WEB_LANGUAGE` | `language` | string | sidebar switch, per browser | Fallback UI locale — `en` or `de` |
| `EASYWALL_WEB_UPDATE_CHECK` | `update_check` | bool | — | Ask github.com once a day whether a newer release exists |
| `EASYWALL_WEB_DEMO_MODE` | `demo_mode` | bool | — | Run against an in-memory mock instead of the core — the public demo only |
| `EASYWALL_WEB_TELEMETRY` | `telemetry` | bool | **System** | Report this installation once a day — off unless set |

The language switch in the sidebar sets a cookie for the browser you are reading
in. It does not write `web.toml`, so it neither overrides `EASYWALL_WEB_LANGUAGE`
nor is overridden by it: the variable decides the language a browser with no
cookie sees.

## `TZ`

Not in either table above because it is not read by easywall at all — the Go
runtime reads it through tzdata, the same as any other Go binary. It sets the
zone the interface renders timestamps in: the audit log, `Applied` and
`Confirmed` times, everything with a clock on it. Without it a container runs
UTC, and an operator in another zone reads every entry offset by their own.

## What you cannot set here, and why

The rule in one sentence: the environment configures where easywall runs; the
interface configures what the firewall does.

- **Rule settings and the acceptance window** — every `[firewall]` switch and
  limit, `[ipv6]`, `[docker]`, `[routing]`, `acceptance.duration` — are written
  by the interface. A variable overriding one of these would be undone the next
  time the container restarts and rereads `easywall.toml`, silently, with the
  interface having reported the change as saved.
- **Credentials, the session key, the TOTP secret, the recovery codes** —
  `username`, `password`, `session_key`, `totp_secret`, `recovery_codes` — are
  secrets, and an environment variable is not one: it is visible to
  `docker inspect`, to anything that reads `/proc/<pid>/environ`, and to
  whatever log somebody pastes into an issue. `web.toml` is `0600`; the
  environment of a running container is not.
- **Being counted** — `telemetry` — *is* settable here, and it is the one key the
  interface also writes. The public demo is configured entirely from its
  environment and reports like any other installation. An answer given in the
  interface is a stored value and beats the variable; **Reset to the environment
  value** on the System page is the way back.

## Behaviour

An empty variable counts as unset — `-e EASYWALL_WEB_LANGUAGE=` leaves the
file's value alone rather than blanking it. A boolean variable is read with
`strconv.ParseBool`, so `1`, `t`, `T`, `true`, `TRUE` and `True` all mean true
and `0`, `f`, `F`, `false`, `FALSE` and `False` all mean false; anything else
stops the process at startup, with the variable named in the error. Prefer
`true` and `false` — the rest are accepted, not recommended.

## Compose example

Based on the repository's own `docker-compose.yml`, with a handful of the
variables above added:

```yaml
services:
  easywall:
    image: ghcr.io/jp1337/easywall:latest
    container_name: easywall
    restart: unless-stopped

    environment:
      - TZ=${TZ:-UTC}
      - EASYWALL_CORE_DATA_DIR=/var/lib/easywall
      - EASYWALL_WEB_BIND_ADDR=0.0.0.0:12227
      - EASYWALL_WEB_LANGUAGE=de
      - EASYWALL_WEB_UPDATE_CHECK=false

    network_mode: host
    cap_add:
      - NET_ADMIN
    security_opt:
      - no-new-privileges:true

    volumes:
      - ./config:/etc/easywall
      - easywall_data:/var/lib/easywall
      - easywall_logs:/var/log/easywall

volumes:
  easywall_data:
  easywall_logs:
```
