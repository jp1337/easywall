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
| Rule or command injection | Every typed rule goes to the kernel as a Go struct over netlink — no shell, no argv, nothing to escape. Custom rules are the one exception and are handled below |
| Escalation from the web process | It has no kernel access. Reaching nftables needs a command the typed protocol accepts |
| Auth brute force | Argon2id, plus 5 attempts per 10 minutes per source address |
| CSRF | Go 1.25 `net/http.CrossOriginProtection` — `Origin` and `Sec-Fetch-Site` on every unsafe method |
| XSS | `html/template` escapes by default; CSP with no `'unsafe-inline'` and no external origin |
| Session hijacking | HTTPS only, `HttpOnly`, `Secure`, `SameSite=Lax`, 600-second lifetime, and every session ends the moment the password changes |
| Locking the admin out | The acceptance window rolls back on its own; if it already has and you are still shut out, `easywall-core panic` reaches the firewall from the console — see [Panic mode](#panic-mode) |
| Known CVEs in dependencies | `govulncheck` on every pull request and weekly, plus CodeQL and `gosec` |
| Dependency hijacking | Renovate raises every update, patch releases auto-merge only once CI is green, minor and major wait for a person; plus secret scanning and dependency review |
| A spoofed source address | `X-Forwarded-For` is **not** trusted — see [behind a reverse proxy](#behind-a-reverse-proxy) |

## Authentication

| | |
|---|---|
| Hash | Argon2id — 64 MiB, 3 iterations, parallelism 4, 16-byte salt per password |
| Default password | none. The first-run wizard is mandatory |
| Rate limit | 5 attempts, refilling one every 2 minutes, per source address |
| Session | 600 s · `HttpOnly` · `Secure` · `SameSite=Lax` |
| Cookie signing key | generated on first start unless configured. Anyone holding it can forge a session, and the placeholder in the sample config is published here — so a missing, short or placeholder key is replaced and written back |
| Logout | ends that session immediately, and only that one. The identifier is recorded as revoked, because a signed cookie is self-contained and telling the browser to drop it leaves the value working. The record is in memory: a restart within ten minutes forgets it |
| Password change | ends every **other** session at once. Each carries a fingerprint of the password hash it was issued under and is refused once that stops matching |
| Recovery | none by design — no mail, no outside service. [Clear the password line]({{ '/docs/installation/first-run/' | relative_url }}#if-you-lose-the-password) on the host |
| Second factor | optional, per the single account — [TOTP and eight recovery codes]({{ '/docs/features/two-factor/' | relative_url }}). Enabling or disabling one ends every other session, the same way a password change does |

With a second factor enrolled, the password step ends in a redirect rather than a
session, and the code is checked at `/login/verify`. That step has no rate limit
of its own and does not need one: three code attempts per intermediate state,
five password rounds per ten minutes per address, so **fifteen code attempts per
ten minutes per address** against a target that rotates every thirty seconds.
`TestLoginVerify_TheSixteenthCodeAttemptDoesNotGetThrough` is that sentence as an
executable claim.

## Panic mode

Since 2.7, `easywall-core` puts the last confirmed rule set back into the kernel
at startup — a reboot no longer empties the firewall the way it used to, and no
longer works as an accidental way back in. `easywall-core panic` is the
deliberate one that replaces it: a console command that takes the firewall down
immediately, whether or not the web interface is reachable at all.

It is a new way to disable the firewall, in full, from the console — and that
belongs on this page whether or not it feels like a feature. Two things about it
matter for the rest of this model:

- **It survives a restart on purpose.** Panic mode is recorded in a marker file
  and the startup restore refuses to run while it exists — otherwise the next
  reboot would put the very rules back that panic mode exists to remove.
- **While the marker exists, an apply is refused and the acceptance rollback
  stops at the kernel.** The stored rules are still reverted — an apply nobody
  confirmed never gets to keep `Current`, or the next restore would install it
  with no window of its own — but the previous rules are not written back into a
  table the console has deliberately torn down. That kernel half is the one place
  in easywall where the central promise, every apply reverts itself unless you
  confirm it, is switched off outright, because there is nothing running to roll
  back onto. Both take effect the instant the marker is written and end the
  instant `easywall-core resume` clears it.

Ending panic mode is console-only, without exception: the banner the interface
shows carries no button. A control there would let the process reachable from
the network re-arm a firewall a human just disarmed at the machine — on
purpose, possibly using a stolen session. Full detail, real output, and the
marker's path: [Recovery & Panic Mode]({{ '/docs/features/recovery/' | relative_url }}).

## Behind a reverse proxy

easywall terminates TLS itself and does **not** believe `X-Forwarded-For` —
unless the peer sending the request is on `trusted_proxies`, a list configured
explicitly and never a boolean — and never believes `X-Real-IP` or
`True-Client-IP`. A client that can set its own source address and is not on
that list walks straight past the login rate limiter.

| | |
|---|---|
| What stays authoritative | `r.RemoteAddr` — the actual TCP peer, unless it is a listed proxy |
| The cost of an untrusted proxy | every request looks like it comes from the proxy, so the limit of five attempts per ten minutes is shared by everyone. One person getting it wrong repeatedly locks the rest out until it refills |
| What to do about it | list the proxy in `trusted_proxies` — [Behind a reverse proxy]({{ '/docs/installation/reverse-proxy/' | relative_url }}) is the whole task, with nginx and Docker |

This only affects the *login* limiter. The firewall's own protection modules count
per source address in the kernel and are unaffected by any HTTP header.

## Transport

HTTPS only, TLS 1.2+. No plaintext port is opened at all. Without a configured
certificate easywall generates a self-signed **ECDSA P-256** one into `ssl_dir`, and
replaces it once it comes within 30 days of expiry — at startup, and twice a day while
the service is running. The certificate is read per handshake rather than once at
startup, so a renewal takes effect without a restart. That matters for a service that
may well outlive its own one-year certificate.

A certificate you configure yourself is never overwritten. It is re-read when the file
changes, so an ACME client renewing it in place needs no restart either.

```toml
[tls]
cert = "/etc/letsencrypt/live/example.com/fullchain.pem"
key  = "/etc/letsencrypt/live/example.com/privkey.pem"
```

### The one place a string reaches a command

Every typed rule reaches the kernel as a Go struct over netlink. **Custom rules are
the exception**: the netlink library takes typed expressions rather than text, so a
statement you write by hand is applied by putting
`add rule inet easywall input <your rule>` into `nft -f -`. Saying "no subprocess
in the apply path", as this page once did, was not true.

| | |
|---|---|
| The risk | `nft` reads a newline and a semicolon as the end of one command and the start of the next, so a rule carrying either is a **second command run by the root daemon** |
| Demonstrated | not theoretical — an imported rule containing a newline wrote into a neighbouring table on a real kernel |
| The mitigation | both characters refused on save, on import, and again inside the core, by the same check |
| Why on the *shape* | a check that parsed nftables would depend on nft's grammar, on a subprocess being available, and on the syntax-check wrapper happening to be balanced. This one depends on none of them |
| Also bounded | 256 statements per check |

What remains is what the feature is for: an operator with an account can write
firewall rules, which is also true of every other page.

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

### Every request that goes out

Two, and this is the whole list.

| | Destination | When | Carries | Default |
|---|---|---|---|---|
| Update check | `api.github.com` | once a day | nothing about you — a plain GET for the newest release | **on**, `update_check = false` removes it |
| Installation count | `telemetry.wdkro.de` | once a day | a random identifier generated on your machine, and the version | **off** until you switch it on |

Neither is on the path of a page, and on a host with no route out both simply fail
and nothing else changes. The exact request the count makes is printed verbatim
under [Configuration]({{ '/docs/configuration/' | relative_url }}#counting-installations).

> **Fixed in v2.4.0.** htmx was configured through a listener for an `htmx:config`
> event, which htmx does not emit — so `allowEval` stayed at its default of `true`
> and the script nonce was never applied. It goes through the
> `meta[name=htmx-config]` tag now. Found by tightening `style-src`, which surfaced
> an inline `<style>` block htmx had been injecting unnoticed.

## What the audit log actually records

One JSON object per line in `<log_dir>/audit.log`, rotated daily, 30 days kept:

```json
{"time":"2026-08-04T14:25:13Z","action":"apply_started","rule_type":"all","detail":"","user":"web"}
{"time":"2026-08-04T14:25:43Z","action":"apply_accepted","rule_type":"all","detail":"","user":"web"}
{"time":"2026-08-04T14:30:00Z","action":"apply_rolledback","rule_type":"all","detail":"timeout","user":"web"}
```

`user` is always `web`, whichever account signed in: the socket protocol carries
no identity yet. It names the process, not the person — see the
[roadmap]({{ '/docs/roadmap/' | relative_url }}).

| Recorded | **Not** recorded |
|---|---|
| `apply_started` · `apply_accepted` · `apply_rolledback` · `apply_failed` | the source address of a change |
| `rollback_failed` — new rules did not take *and* the old ones did not return | which account made the change |
| `rules_saved` · `rules_imported` | |
| `options_saved` · `settings_saved` · `system_saved` | |

> **Since 2.8, logins are in the audit log.** Nine events — signed in, sign-in
> failed, second factor failed, a recovery code used, sign-in attempts blocked,
> signed out, and the second factor switched on, off or regenerated — see
> [the nine login events]({{ '/docs/features/audit-log/' | relative_url }}#the-nine-login-events).
> None of them carries colour: a sign-in does not move the firewall.

Reading it: [Audit log]({{ '/docs/features/audit-log/' | relative_url }}).

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

| | |
|---|---|
| A compromised root account | root owns the core |
| A kernel nftables vulnerability | that is below easywall entirely |
| An administrator writing a bad rule | the [audit log]({{ '/docs/features/audit-log/' | relative_url }}) records it; nothing prevents it |
| Anyone holding `session_key` | it signs the cookies, so it *is* a login. Keep it out of backups you share |

## Reporting a vulnerability

**Not as a public issue.** Use
[GitHub Security Advisories](https://github.com/jp1337/easywall/security/advisories/new)
for private disclosure — see [SECURITY.md](https://github.com/jp1337/easywall/blob/main/SECURITY.md).
