---
layout: default
title: Roadmap
description: Ten releases, ordered by exposure — comprehension first now the holes are closed, then maintenance, then reach.
---

# Roadmap

Correctness first: a firewall that quietly does less than it says is worse than
one that does less and says so. What follows is planned in this order, not
promised in it. It gets corrected when something changes rather than quietly
ageing, which is exactly the failure the version before this one demonstrated.

**Ordering principle: by exposure.** The two holes an attacker could actually
walk through came first and shipped in 2.7 and 2.8 — see the
[Changelog]({{ '/docs/changelog/' | relative_url }}). What is left helps you
understand what you are doing, then lets you maintain it, then reaches further:

```
Understand what you do   2.14  You can see it working
                          2.15  When something happens, you hear about it

Be able to maintain it   2.16  Every entry has a why and an until
                          2.17  Whoever knocks gets locked out
                          2.18  Other people's lists, and countries

Reach further            2.19  Outbound traffic
                          2.20  More than one account
                          2.21  Eight languages
                          3.0   Reachable from outside
                          3.1   Passkeys, as a second factor
```

One theme per release, sayable in one sentence — the changelog heading then
writes itself. A model change travels with the feature that justifies it, never
earlier as an end in itself and never twice.

> **Amended in 2.12.** Two releases were inserted here and everything below
> them moved back two places. **3.0 and 3.1 kept their numbers**: those are
> statements rather than positions. 3.0 is a major because an API is a second
> public interface and a compatibility promise. 3.1 follows 3.0 because
> WebAuthn needs a registrable hostname. Both stay true however many 2.x
> releases come first. 2.13 also takes the trusted-proxy list out of 3.0's row,
> where it had been a clause.

> **Amended in 2.8.** Passkeys were a clause inside 3.0's row; they are now
> 3.1, their own entry, sitting after 3.0 rather than beside it. WebAuthn
> requires a registrable domain as its Relying Party ID, which most easywall
> installations do not have.

| Version | What | Why it comes when it does |
|---|---|---|
| **2.14** | **You can see it working** — a counter on every rule, reset on each apply | An open port nobody uses is the most common avoidable exposure on a hobby server, and nobody finds it because nobody goes looking |
| **2.15** | **When something happens, you hear about it** — a webhook or ntfy push for a rollback, a confirmed apply, panic mode, repeated failed logins | The core still never opens a connection outward; the web process polls the audit log and sends the notification, the same separation as everything else |
| **2.16** | **Every entry has a why and an until** — blacklist entries carry a comment and an expiry | The textarea becomes a table; pasting a list of addresses still works, folded underneath it |
| **2.17** | **Whoever knocks gets locked out** — repeated knocking on closed ports blocks itself, in an nftables set with a timeout, no userspace parser involved | Substitutes for reading `journald`/`auth.log` as root. A named set that fail2ban or CrowdSec can write into covers the credential case without turning the root process into a log parser |
| **2.18** | **Other people's lists, and countries** — curated blocklists and country zones, each switched on individually, all off by default | The web process downloads, never the core. A feed is consulted after the whitelist, unlike your own blacklist — ten thousand entries from someone else's hand should not be able to lock you out of your own address |
| **2.19** | **Outbound traffic** — what the server may send out becomes configurable, `open` (today's behaviour) or `allowlist` | The output chain has policy `ACCEPT` and not one rule today. Highest lockout risk on this list; gets its own acceptance-window round and its own veth proof |
| **2.20** | **More than one account** — the `user` field the protocol has never carried, plus an observer role that can see but not apply | `WriteAuditLog` already takes a user; nothing upstream of it has one to give. Every audit entry has said `web` since it existed |
| **2.21** | **Eight languages** — Spanish, Portuguese (BR), Italian, Dutch, Polish, Russian, Chinese (Simplified), Japanese | One pass, once the string set is stable. No RTL: that is a design-system change, not a translation |
| **3.0** | **Reachable from outside** — a REST API with token auth and ACME as an alternative to a reverse proxy | A major version because an API is a second public interface and a compatibility promise easywall has not made before |
| **3.1** | **Passkeys, as a second factor** — WebAuthn alongside TOTP, never in place of the password | Wait for 3.0 on purpose: WebAuthn requires a registrable domain as its Relying Party ID and **rejects a bare IP address**, which is how most easywall installations are reached (`https://192.168.1.10:12227`) — passkeys cannot come before a real hostname and certificate exist |

## Deliberately excluded

| | |
|---|---|
| SMTP notifications | Credentials in `web.toml`, foreign mail servers, deliverability — ntfy reaches the same phone without any of it |
| A reimplemented fail2ban | Replaced by the named set the real one can write into (2.17) |
| Zones, on the firewalld model | easywall runs on hosts with one uplink; what zones would be for is covered by `routing.mode` |
| Rule schedules | "Open this port between 08:00 and 18:00" is a state machine nobody can debug once it is in the wrong state |
| IDS/IPS, deep packet inspection, QoS | Different products. easywall filters packets |
| Managing several hosts from one interface | The API in 3.0 makes Ansible possible. A fleet interface is a second product |

## What already shipped

Every release, newest first, each with what it was for:
[Changelog]({{ '/docs/changelog/' | relative_url }}).

---

**Next:** [Architecture]({{ '/docs/architecture/' | relative_url }}) ·
[Security]({{ '/docs/security/' | relative_url }}) ·
[Contributing]({{ '/docs/contributing/' | relative_url }})
