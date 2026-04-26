---
layout: default
title: Security Model
description: easywall's layered security architecture — two-process isolation, Argon2id auth, nftables netlink API, and audit trail.
---

# Security Model

easywall was rebuilt from scratch with security as the primary design constraint. Every architectural decision traces back to a specific threat or past vulnerability.

## Threat Model

| Threat | Mitigation |
|---|---|
| Rule/command injection | Direct netlink API — no subprocess, no string interpolation |
| Privilege escalation from web | Web process has no root access; talks to core via typed socket protocol |
| Auth brute force | Rate limiting (5 attempts / 10 min per IP), Argon2id password hashing |
| CSRF | `gorilla/csrf` on all POST endpoints — missing token returns HTTP 403 |
| XSS | Go `html/template` auto-escapes all output by default; Content-Security-Policy header |
| Session hijacking | HTTPS-only, HttpOnly, SameSite=Lax cookies; 600-second session lifetime |
| Admin lockout | Two-step activation: rules auto-roll back if not confirmed within timeout |
| Audit trail gaps | Structured JSON entries logged for every config change and apply event |
| Known CVEs | `govulncheck` in CI — weekly + every PR — with GitHub Security Advisories |
| Dependency hijacking | Dependabot, GitHub Secret Scanning, Dependency Review (blocks critical CVE PRs) |
| Sensitive config exposure | `session_key`, `csrf_key`, and `password` live only in root-owned config files |

---

## Process Isolation

<div class="mermaid">
flowchart TB
    Browser["🌐 Browser\nHTTPS :12227"]

    subgraph web["easywall-web  (user: easywall, no root)"]
        W1["• No CAP_NET_ADMIN"]
        W2["• No direct kernel access"]
        W3["• Reads config + TLS cert"]
        W4["• Binds 0.0.0.0:12227"]
    end

    subgraph core["easywall-core  (root / CAP_NET_ADMIN)"]
        C1["• Owns /run/easywall/core.sock (mode 0660, group easywall)"]
        C2["• Reads/writes /var/lib/easywall/rules.json"]
        C3["• Writes /var/log/easywall/audit.log"]
        C4["• No inbound network port"]
    end

    Kernel["🐧 nftables  (table inet easywall)\nvia direct netlink — no nft subprocess"]

    Browser -->|"HTTPS TLS 1.2+"| web
    web -->|"Unix socket — JSON commands only"| core
    core -->|"netlink syscalls"| Kernel
</div>

The web process communicates with the core exclusively via a Unix socket. It cannot invoke shell commands, cannot access the filesystem of the core process, and has no privileges to modify nftables directly.

---

## IPC Protocol

All commands from web → core are Go structs serialised as JSON. The protocol defines exactly 9 command types:

| Command | Effect |
|---|---|
| `GET_STATUS` | Returns current firewall status |
| `GET_RULES` | Returns the current rule state |
| `SAVE_RULES` | Stages a rule change (one rule type at a time) |
| `APPLY_RULES` | Starts the apply + acceptance flow |
| `ACCEPT` | Confirms the applied rules |
| `GET_OPTIONS` | Returns firewall option flags |
| `EXPORT_RULES` | Returns the full rule state as JSON |
| `IMPORT_RULES` | Validates and stages an imported rule set |
| `UNKNOWN` / any other | Returns an error — nothing is executed |

The core never executes arbitrary strings. nftables rules are constructed via the `github.com/google/nftables` netlink library directly — zero shell involvement, zero string interpolation.

---

## Two-Step Activation

Every rule change must pass through two-step activation before becoming permanent:

```
User clicks Apply
       │
       ▼
Core backs up current rules
Core applies new rules to nftables
Core starts acceptance timer (default: 120 s)
       │
       ├── User confirms in web UI within window ──► Rules become permanent
       │
       └── Timeout expires ──────────────────────► Auto-rollback to backup
```

This prevents the most common firewall administration risk: accidentally locking yourself out by applying rules that block your own connection. Even if your SSH session drops, the old rules are restored after the timeout.

---

## Authentication

- **Algorithm**: Argon2id via `golang.org/x/crypto/argon2`
- **Parameters**: memory=65536 KB, iterations=3, parallelism=4 (tunable)
- **No default password** — the first-run wizard is mandatory on first access
- **Login rate limiting**: max 5 attempts per 10 minutes per source IP (`golang.org/x/time/rate`)
- **Session lifetime**: 600 seconds; cookie is `HttpOnly`, `Secure`, `SameSite=Lax`

---

## Transport Security

The web process listens exclusively on HTTPS (TLS 1.2+). No HTTP redirect port is opened. On first start, if no custom certificate is configured, a self-signed RSA-4096 / ECDSA P-256 certificate is generated and stored in the configured `ssl_dir`. The certificate is automatically renewed when it expires within 30 days.

For production use, configure a certificate from a trusted CA (Let's Encrypt or enterprise CA) via `web.toml`:

```toml
[tls]
cert = "/etc/letsencrypt/live/example.com/fullchain.pem"
key  = "/etc/letsencrypt/live/example.com/privkey.pem"
```

---

## Audit Log

All security-relevant events are written to `/var/log/easywall/audit.log` as structured JSON:

```json
{"time":"2026-04-26T14:23:01Z","event":"login_success","user":"admin","ip":"203.0.113.42"}
{"time":"2026-04-26T14:25:13Z","event":"apply_started","user":"admin","scope":"all"}
{"time":"2026-04-26T14:25:43Z","event":"apply_accepted","user":"admin","scope":"all"}
{"time":"2026-04-26T14:30:00Z","event":"apply_rolledback","user":"admin","scope":"all","reason":"timeout"}
{"time":"2026-04-26T14:31:05Z","event":"login_failed","ip":"198.51.100.7"}
```

Event types: `login_success`, `login_failed`, `logout`, `apply_started`, `apply_accepted`, `apply_rolledback`, `rules_saved`, `rules_imported`, `firstrun_complete`.

Audit logs are rotated by logrotate (configured in `/etc/logrotate.d/easywall`) — daily, 30 days retained, compressed.

---

## CVE History

easywall v0.3.1 (Python/Flask) received a CVE due to:

- Web process ran as root — one injection = full system compromise
- File-based IPC via sentinel files — race conditions possible
- `nft` / `iptables` invoked via `subprocess.run()` with partially-user-controlled strings
- SHA512 with hostname as salt — trivially reversible with knowledge of hostname

v2 addresses every one of these root causes architecturally:

| v1 Problem | v2 Fix |
|---|---|
| Web ran as root | Web runs as unprivileged `easywall` user — kernel access is impossible |
| Shell subprocess for iptables | `github.com/google/nftables` — direct netlink, no subprocess |
| File-based IPC | Typed Unix socket protocol — no race conditions, no ambiguity |
| SHA512 + hostname salt | Argon2id with proper parameters |
| Flask without CSRF | gorilla/csrf on every POST |

---

## Reporting Security Issues

**Do not open a public GitHub issue for security vulnerabilities.**

Use [GitHub Security Advisories](https://github.com/jp1337/easywall/security/advisories/new) for private disclosure.

- Initial response: ≤ 48 hours
- Patch target for critical issues: ≤ 14 days
- Credit given for responsible disclosure in release notes
