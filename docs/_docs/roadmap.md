---
layout: default
title: Roadmap
description: Fifteen releases, ordered by exposure — the holes first, then comprehension, then reach.
---

# Roadmap

Correctness first: a firewall that quietly does less than it says is worse than
one that does less and says so. What follows is planned in this order, not
promised in it — it gets corrected when something changes rather than quietly
ageing, which is exactly the failure the version before this one demonstrated.

**Ordering principle: by exposure.** The two holes an attacker could actually
walk through come first, then the releases that help you understand what you
are doing, then the ones that let you maintain it, then reach:

```
Close the exposure       2.7   The firewall survives a reboot
                          2.8   A second factor

Understand what you do   2.9   The interface speaks French — and the next
                                language need not come from us
                          2.10  What changes is on the screen
                          2.11  A rule names a service and who may reach it
                          2.12  You can see it working
                          2.13  When something happens, you hear about it

Be able to maintain it   2.14  Every entry has a why and an until
                          2.15  Whoever knocks gets locked out
                          2.16  Other people's lists, and countries

Reach further            2.17  Outbound traffic
                          2.18  More than one account
                          2.19  Nine languages
                          3.0   Reachable from outside
                          3.1   Passkeys, as a second factor
```

One theme per release, sayable in one sentence — the changelog heading then
writes itself. A model change travels with the feature that justifies it, never
earlier as an end in itself and never twice.

> **Amended in 2.8.** The i18n item was one entry late on this list; it is now
> two, and the first of them is early. French ships in-house and **visibly marked
> as unreviewed**, in the switcher and in the coverage report — the mechanism has
> to be able to express "present, but not checked by a human" anyway, or a
> contributed language cannot be either.

> **Amended in 2.8.** Passkeys were a clause inside 3.0's row; they are now
> 3.1, their own entry, sitting after 3.0 rather than beside it — WebAuthn
> requires a registrable domain as its Relying Party ID, which most easywall
> installations do not have.

| Version | What | Why it comes when it does |
|---|---|---|
| **2.7** | **The firewall survives a reboot** — after a restart the rules that were last confirmed are in force again, with no acceptance window | Before this, a reboot emptied nftables and was the accidental way back into a machine your own rules had shut you out of. `easywall-core panic` replaces that on purpose, from the console — see [Recovery & Panic Mode]({{ '/docs/features/recovery/' | relative_url }}) |
| **2.8** | **A second factor** — TOTP plus one-time recovery codes, and login events finally in the audit log | A stolen password alone opens everything today. The way back stays a line in `web.toml` — a second factor that needs a second emergency exit is not one |
| **2.9** | **The interface speaks French — and the next language need not come from us** — parity becomes fallback for everything except `en`/`de`, which stay hard; a coverage report per language, shown rather than hidden; a guide in `CONTRIBUTING.md`; the language switcher rebuilt as a `<select>` plus a submit button, progressively enhanced; and `fr.json` as the proof the mechanism carries, shipped marked as unreviewed | A fallback mechanism with no third language is stock — it changes nothing while only `en` and `de` exist, and a guard relaxed before anyone needs the relaxation is a guard nobody will notice later. So French travels with it, which gives it its justification and brings the language that was asked for forward by about a year. The switcher goes with them: two chip buttons in a 240 px sidebar do not hold eleven endonyms |
| **2.10** | **What changes is on the screen** — the difference between staged and current, before you press Apply, with a warning if it cuts the connection you are using | No protocol work needed; the data is already there. The warning is the real gain — knowing before the 120 seconds, not during them |
| **2.11** | **A rule names a service and who may reach it** — a curated catalogue ("Pi-hole", "Home Assistant") with a suggested source, and a free-text option that stays | A catalogue entry without a source restriction is wrong for the interesting cases; shipping them apart would mean rewriting every entry a release later |
| **2.12** | **You can see it working** — a counter on every rule, reset on each apply | An open port nobody uses is the most common avoidable exposure on a hobby server, and nobody finds it because nobody goes looking |
| **2.13** | **When something happens, you hear about it** — a webhook or ntfy push for a rollback, a confirmed apply, panic mode, repeated failed logins | The core still never opens a connection outward; the web process polls the audit log and sends the notification, the same separation as everything else |
| **2.14** | **Every entry has a why and an until** — blacklist entries carry a comment and an expiry | The textarea becomes a table; pasting a list of addresses still works, folded underneath it |
| **2.15** | **Whoever knocks gets locked out** — repeated knocking on closed ports blocks itself, in an nftables set with a timeout, no userspace parser involved | Substitutes for reading `journald`/`auth.log` as root. A named set that fail2ban or CrowdSec can write into covers the credential case without turning the root process into a log parser |
| **2.16** | **Other people's lists, and countries** — curated blocklists and country zones, each switched on individually, all off by default | The web process downloads, never the core. A feed is consulted after the whitelist, unlike your own blacklist — ten thousand entries from someone else's hand should not be able to lock you out of your own address |
| **2.17** | **Outbound traffic** — what the server may send out becomes configurable, `open` (today's behaviour) or `allowlist` | The output chain has policy `ACCEPT` and not one rule today. Highest lockout risk on this list; gets its own acceptance-window round and its own veth proof |
| **2.18** | **More than one account** — the `user` field the protocol has never carried, plus an observer role that can see but not apply | `WriteAuditLog` already takes a user; nothing upstream of it has one to give. Every audit entry has said `web` since it existed |
| **2.19** | **Nine languages** — Spanish, Portuguese (BR), Italian, Dutch, Polish, Russian, Chinese (Simplified), Japanese | One pass, once the string set is stable. No RTL: that is a design-system change, not a translation |
| **3.0** | **Reachable from outside** — a REST API with token auth, ACME as an alternative to a reverse proxy, and a trusted-proxy *list* rather than a boolean | A major version because an API is a second public interface and a compatibility promise easywall has not made before |
| **3.1** | **Passkeys, as a second factor** — WebAuthn alongside TOTP, never in place of the password | Wait for 3.0 on purpose: WebAuthn requires a registrable domain as its Relying Party ID and **rejects a bare IP address**, which is how most easywall installations are reached (`https://192.168.1.10:12227`) — passkeys cannot come before a real hostname and certificate exist |

## Deliberately excluded

| | |
|---|---|
| SMTP notifications | Credentials in `web.toml`, foreign mail servers, deliverability — ntfy reaches the same phone without any of it |
| A reimplemented fail2ban | Replaced by the named set the real one can write into (2.15) |
| Zones, on the firewalld model | easywall runs on hosts with one uplink; what zones would be for is covered by `routing.mode` |
| Rule schedules | "Open this port between 08:00 and 18:00" is a state machine nobody can debug once it is in the wrong state |
| IDS/IPS, deep packet inspection, QoS | Different products. easywall filters packets |
| Managing several hosts from one interface | The API in 3.0 makes Ansible possible. A fleet interface is a second product |

## Done in 2.6

| | |
|---|---|
| Proof, not counts | `nftables_semantics_test.go` reads back `nft list table` and asserts verdicts, direction and evaluation order instead of a rule count; `nftables_forward_test.go` routes a real packet through the forward chain across a veth pair. A rule that dropped where it should accept used to pass — a count cannot see what a rule matches |
| An opt-in count of installations | sends one request a day to `telemetry.wdkro.de` — a random identifier and the version, nothing else — and stays off until switched on. Withdrawable from the System page without needing the core reachable. Detail: [Security]({{ '/docs/security/' | relative_url }}#every-request-that-goes-out) |
| The nine firewall limits | one table now carries the range for each; out of range from the interface is refused, out of range in the file is clamped and said out loud — a value too large used to wrap a 32-bit nftables field instead of failing |
| The config file | ships as a template, not a dpkg conffile, so an upgrade that also changes the shipped default no longer stalls at a prompt with the old processes still serving |
| The import timeout | matches what the core actually spends on `nft --check`, so a slow check is no longer reported to the operator as a failed import |
| Signing out | is a `POST`, inside the same CSRF protection every other state change already had |
| The post-incident snapshot | attributes each chain to its own table family, instead of crediting one table with another's chains |
| What counts as a network | one definition now, shared by the editor, the core and the demo |
| The Debian package | exists for arm64, built and verified on a runner of its own architecture |
| `--write-config` | the flag the documentation had already promised, on both binaries |
| The documentation | split into a published site and a maintainers-only set of pages, all 22 existing pages checked claim by claim, four pages added for parts of the interface that had none |

## Done in 2.5

| | |
|---|---|
| Rate limits | counted per source address — four modules held one counter for the whole machine, so an attacker could spend the budget and lock everyone else out |
| The options page | every switch reaches the firewall; 17 of 31 did not |
| Port forwarding | goes the direction it says |
| Invalid rules | refused instead of silently skipped |
| The dashboard | "rules are live" is asked of the kernel rather than assumed |
| The Debian package | contains its binaries and is published on the release |

---

**Next:** [Architecture]({{ '/docs/architecture/' | relative_url }}) ·
[Security]({{ '/docs/security/' | relative_url }}) ·
[Contributing]({{ '/docs/contributing/' | relative_url }})
