---
layout: default
title: Security Model
description: What the design defends against, what it does not, and the CVE in the Python version that shaped it.
---

# Security Model

The whole design follows from one decision: the process reachable from the network
holds no privilege worth stealing.

{% include themed-figure.html base="/assets/diagrams/architecture" ext="svg"
   alt="Browser talks HTTPS to easywall-web, which runs unprivileged; easywall-web talks typed JSON over a Unix socket to easywall-core, which runs as root and speaks netlink to the nftables table inet easywall." %}

## Threat model

| Threat | Mitigation |
|---|---|
| Rule or command injection | netlink API — no subprocess and no string interpolation in the apply path |
| Escalation from the web process | It has no kernel access. Reaching nftables needs a command the typed protocol accepts |
| Auth brute force | Argon2id, plus 5 attempts per 10 minutes per source address |
| CSRF | Go 1.25 `net/http.CrossOriginProtection` — `Origin` and `Sec-Fetch-Site` on every unsafe method |
| XSS | `html/template` escapes by default; CSP with no `'unsafe-inline'` and no external origin |
| Session hijacking | HTTPS only, `HttpOnly`, `Secure`, `SameSite=Lax`, 600-second lifetime |
| Locking the admin out | The acceptance window rolls back on its own |
| Known CVEs in dependencies | `govulncheck` on every PR and weekly |
| Dependency hijacking | Dependabot, secret scanning, dependency review |

## Authentication

| | |
|---|---|
| Hash | Argon2id — 64 MiB, 3 iterations, parallelism 4, 16-byte salt per password |
| Default password | none. The first-run wizard is mandatory |
| Rate limit | 5 attempts, refilling one every 2 minutes, per source address |
| Session | 600 s · `HttpOnly` · `Secure` · `SameSite=Lax` |
| Recovery | none by design — no mail, no outside service. Editing `web.toml` on the host is the only way back |

## Transport

HTTPS only, TLS 1.2+. No plaintext port is opened at all. Without a configured
certificate easywall generates a self-signed **ECDSA P-256** one into `ssl_dir`, and
renews it when it is within 30 days of expiry.

```toml
[tls]
cert = "/etc/letsencrypt/live/example.com/fullchain.pem"
key  = "/etc/letsencrypt/live/example.com/privkey.pem"
```

### Nothing is loaded from a third party

Fonts, stylesheet, icons and htmx are served by easywall itself, and the policy
permits no external origin:

```
default-src 'self'; script-src 'self' 'nonce-<per-request>'; style-src 'self';
font-src 'self'; img-src 'self' data:; connect-src 'self'
```

Two reasons, both practical. An administrative interface should not report a visit
to anyone — the earlier build loaded its typefaces from Google Fonts, which did
exactly that. And easywall often runs on hosts with no outbound route, where that
request simply failed and left the typography broken on the machines the tool is
built for.

> **A constraint on contributions.** `style-src` has no `'unsafe-inline'`, so
> assigning `element.style.*` from JavaScript, or letting a library inject a
> `<style>` block, is blocked. Scripts toggle a class instead.

> **Fixed in v2.4.0.** htmx was configured through a listener for an `htmx:config`
> event, which htmx does not emit. The listener never ran, so `allowEval` stayed at
> its default of `true` and the script nonce was never applied. Configuration now
> goes through the `meta[name=htmx-config]` tag htmx reads while initialising, with
> `allowEval` and `allowScriptTags` disabled. Found by tightening `style-src`, which
> surfaced the inline `<style>` block htmx had been injecting unnoticed.

## What the audit log actually records

One JSON object per line in `<log_dir>/audit.log`, rotated daily, 30 days kept:

```json
{"time":"2026-08-04T14:25:13Z","action":"apply_started","rule_type":"all","detail":"","user":"admin"}
{"time":"2026-08-04T14:25:43Z","action":"apply_accepted","rule_type":"all","detail":"","user":"admin"}
{"time":"2026-08-04T14:30:00Z","action":"apply_rolledback","rule_type":"all","detail":"timeout","user":"admin"}
```

| Recorded | **Not** recorded |
|---|---|
| `apply_started` · `apply_accepted` · `apply_rolledback` · `apply_failed` | logins, successful or failed |
| `rules_saved` · `rules_imported` | logouts |
| `options_saved` · `settings_saved` · `system_saved` | the source address of a change |

> **Authentication events are not in the audit log.** An earlier version of this page
> listed `login_success`, `login_failed` and `logout` among the event types. Nothing
> writes them, and nothing ever did. For evidence of failed logins use the web
> process's own output — `journalctl -u easywall-web` — where the rate limiter and
> the auth handler log. Recording them in the audit log is a gap, not a feature.

Reading it: [Audit log]({{ '/features/audit-log/' | relative_url }}).

## The CVE that shaped this

easywall v0.3.1 — Python, Flask, `iptables` — was archived in 2022 after a
disclosure. Four root causes, and what replaced each:

| v1 | v2 |
|---|---|
| Web process ran as root — one injection was full compromise | Web runs unprivileged; there is no kernel access to misuse |
| `iptables` through `subprocess` with user-controlled strings | `google/nftables` over netlink — no shell, no argv |
| File-based IPC with sentinel files — racy | A typed protocol over a Unix socket |
| SHA-512 salted with the hostname — trivially reversed | Argon2id with a random 16-byte salt per password |

## What this does not protect you from

- **A compromised root account.** Root owns the core.
- **A vulnerability in the kernel's nftables subsystem.** That is below easywall.
- **A legitimate administrator making a bad rule.** The audit log records it;
  nothing prevents it.

## Reporting a vulnerability

**Not as a public issue.** Use
[GitHub Security Advisories](https://github.com/jp1337/easywall/security/advisories/new)
for private disclosure — see [SECURITY.md](https://github.com/jp1337/easywall/blob/main/SECURITY.md).
