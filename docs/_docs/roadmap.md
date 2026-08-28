---
layout: default
title: Roadmap
description: Twelve releases, ordered by exposure — comprehension first now the holes are closed, then maintenance, then reach.
---

# Roadmap

Correctness first: a firewall that quietly does less than it says is worse than
one that does less and says so. What follows is planned in this order, not
promised in it — it gets corrected when something changes rather than quietly
ageing, which is exactly the failure the version before this one demonstrated.

**Ordering principle: by exposure.** The two holes an attacker could actually
walk through came first and are done — see *Done in 2.7* and *Done in 2.8*
below. What is left helps you understand what you are doing, then lets you
maintain it, then reaches further:

```
Understand what you do   2.12  You can see it working
                          2.13  When something happens, you hear about it

Be able to maintain it   2.14  Every entry has a why and an until
                          2.15  Whoever knocks gets locked out
                          2.16  Other people's lists, and countries

Reach further            2.17  Outbound traffic
                          2.18  More than one account
                          2.19  Eight languages
                          3.0   Reachable from outside
                          3.1   Passkeys, as a second factor
```

One theme per release, sayable in one sentence — the changelog heading then
writes itself. A model change travels with the feature that justifies it, never
earlier as an end in itself and never twice.

> **Amended in 2.8.** Passkeys were a clause inside 3.0's row; they are now
> 3.1, their own entry, sitting after 3.0 rather than beside it — WebAuthn
> requires a registrable domain as its Relying Party ID, which most easywall
> installations do not have.

| Version | What | Why it comes when it does |
|---|---|---|
| **2.12** | **You can see it working** — a counter on every rule, reset on each apply | An open port nobody uses is the most common avoidable exposure on a hobby server, and nobody finds it because nobody goes looking |
| **2.13** | **When something happens, you hear about it** — a webhook or ntfy push for a rollback, a confirmed apply, panic mode, repeated failed logins | The core still never opens a connection outward; the web process polls the audit log and sends the notification, the same separation as everything else |
| **2.14** | **Every entry has a why and an until** — blacklist entries carry a comment and an expiry | The textarea becomes a table; pasting a list of addresses still works, folded underneath it |
| **2.15** | **Whoever knocks gets locked out** — repeated knocking on closed ports blocks itself, in an nftables set with a timeout, no userspace parser involved | Substitutes for reading `journald`/`auth.log` as root. A named set that fail2ban or CrowdSec can write into covers the credential case without turning the root process into a log parser |
| **2.16** | **Other people's lists, and countries** — curated blocklists and country zones, each switched on individually, all off by default | The web process downloads, never the core. A feed is consulted after the whitelist, unlike your own blacklist — ten thousand entries from someone else's hand should not be able to lock you out of your own address |
| **2.17** | **Outbound traffic** — what the server may send out becomes configurable, `open` (today's behaviour) or `allowlist` | The output chain has policy `ACCEPT` and not one rule today. Highest lockout risk on this list; gets its own acceptance-window round and its own veth proof |
| **2.18** | **More than one account** — the `user` field the protocol has never carried, plus an observer role that can see but not apply | `WriteAuditLog` already takes a user; nothing upstream of it has one to give. Every audit entry has said `web` since it existed |
| **2.19** | **Eight languages** — Spanish, Portuguese (BR), Italian, Dutch, Polish, Russian, Chinese (Simplified), Japanese | One pass, once the string set is stable. No RTL: that is a design-system change, not a translation |
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

## Done in 2.11

| | |
|---|---|
| A rule names a service and who may reach it | a port rule carries a source restriction — empty still means everyone — and a catalogue of 29 services fills in its ports and a suggested restriction in one click. The service is a label: an entry corrected later leaves a live rule untouched |
| The SSH brute-force chain stopped outranking the blacklist | it is consulted before the blacklist and used to **accept**, so a blacklisted address could open an SSH connection while it stayed under the rate limit, and port 22 was accepted outright whenever the module was on and no rule opened it. It now returns, and the rest of the chain decides |

## Done in 2.10

| | |
|---|---|
| What changes is on the screen | the staged/current diff, the configuration drift beside it, and a three-way reachability verdict for the connection you are reading it on. The half nobody had noticed was the configuration: options and network settings were in no pending calculation, so `/options` said "apply to activate" while `/apply` said there was nothing to apply, and the false one was on the page with the button |

## Done in 2.9

| | |
|---|---|
| The interface speaks French | `locales/fr.json` carries the whole interface — 463 strings. It shipped in 2.9.0 marked **unreviewed** and was accepted as reviewed shortly after; the state itself stays, because "present, but not checked by a human" is the normal condition of a translation somebody sends in, and it had to exist before a contributed language could |
| Parity became a rule for `en` and `de` alone | Every other language may have gaps. A missing key renders the English string — go-i18n already did that per message ID — and the gap is now *counted* rather than hidden, because the fallback is precisely what makes a gap invisible: the page looks finished, which is right for the operator in front of it and wrong for everybody else. Coverage never rounds up to 100 while anything is missing, and declines to count an empty string or a value byte-identical to the English one, so the number cannot be raised by pasting |
| Why French travelled with the mechanism | Amended in 2.8, delivered here. A fallback rule with no third language changes nothing while only `en` and `de` exist, and a guard relaxed before anybody needs the relaxation is one nobody notices later. French came forward about a year to give it its justification, and 2.19 lost it from that row's count |
| The language switcher | a `<select>` with a submit button instead of two chips: two do not hold eleven endonyms in a 240 px sidebar. It still works with JavaScript switched off — an operator who cannot read the interface should not also need JavaScript to fix that |
| The sentences a translator must not get backwards | `docs-tech/i18n-review.md` collects the thirty-odd where a wrong word changes what the firewall promises — which list is consulted first, what the acceptance window undertakes, what panic mode does not end. One rule: rephrase freely, but do not change what the sentence *claims*. A window that "keeps" a change in one language and "undoes" it in another describes a different product depending on which language you read |
| Twelve environment variables | `EASYWALL_CORE_*` and `EASYWALL_WEB_*`, and the first environment either binary has ever read — before this both processes took only `-config`, so a container had no way to set a socket path or an address to bind without writing a config onto a volume ahead of the first boot. The list stops at deployment: the environment says *where* easywall runs, the interface says *what the firewall does* |
| The release announces itself | one Discord embed from `release.yml` once the assets are up, best-effort. 2.8.0 was announced by hand, hours later. The Ko-fi post is **not** automated and cannot be — Ko-fi has no writing API — and is documented as a person's step rather than left looking like a job that exists |

## Done in 2.8

| | |
|---|---|
| A second factor | RFC 6238 TOTP for the single account, verified against the RFC's own published test vectors, plus eight one-time recovery codes stored argon2-hashed rather than in the clear. Nothing is written until a code confirms the pairing, and `/login/verify` issues no session until the code is right too |
| The way back | stays a line in `web.toml`: clear `totp_secret` and `recovery_codes` on the host, the same file the password already lives in. A second factor that needs a second emergency exit is not one — and it was documented before the release shipped, not after |
| The wizard offers it too | unticked, and skipping is a first-class answer rather than a validation failure: easywall runs on boards with no RTC that come up at the epoch until NTP lands, and TOTP cannot verify against a clock like that |
| Login events | reach the audit log at last — nine of them, from a fixed enum rather than free text, and the three a stranger can trigger without any credential are debounced in the core. `LOG_EVENT` is the eighteenth command the protocol declares and the first the web process sends rather than receives |
| An unauthenticated `POST /logout` | could erase the visible audit log |

## Done in 2.7

| | |
|---|---|
| The firewall survives a reboot | nftables forgets everything on restart, and `nft.Apply` was reachable from exactly two places in the codebase — an apply and its rollback. Not from startup. So every reboot left the machine unfiltered until somebody opened the interface and pressed Apply, on a product whose first sentence is a safety promise; the original Python easywall had the identical gap. The core now puts `Current` into the kernel before its socket accepts a single connection, with **no acceptance window**: `Current` is by definition a set that has already survived one |
| `easywall-core panic` / `resume` / `status` | a console-only way back into a machine your own rules have shut you out of, now that a reboot no longer provides one by accident. The marker deliberately survives a restart, or the next reboot would put back the rules panic mode exists to remove. The banner has **no button**: a control reachable from the network would let a stolen session re-arm a firewall a human disarmed at the machine on purpose |
| Every timestamp was in the wrong zone | the conversion built its comparison in the *stored* zone rather than the viewer's, so "how long ago" and the date-format boundary were both computed against the wrong clock — a change at 23:30 UTC could be attributed to the wrong calendar day. The documented fix, `timedatectl` and a restart, could not have worked. Nothing stored on disk changed: the audit log is still RFC 3339 UTC, byte for byte |
| One mutex | guards every method on `NftablesManager`. `Flush` ships every caller's buffered netlink messages at once and empties the queue regardless of who filled it, so an apply and a panic in flight together could see one call's messages folded into the other's — a rollback reporting success having programmed nothing, or the reverse |
| The panic marker | is re-read *after* every write to the table, not only before one. Checking first is necessary and not sufficient, and cross-process no mutex helps. The loser of that race was a machine filtering while both the banner and `easywall-core status` called it deliberately unfiltered |

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
