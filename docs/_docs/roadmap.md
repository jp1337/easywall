---
layout: default
title: Roadmap
description: Seventeen releases, ordered by exposure — proof and comprehension first now the holes are closed, then maintenance, then reach.
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
Prove it works           2.17  It proves what it says

Prove who you are        2.18  A password alone is not enough

Understand what you do   2.14  The window shows that it is running
                          2.15  You can see it working
                          2.19  When something happens, you hear about it
                          2.20  What it counts can be asked for

Be able to maintain it   2.21  Every entry has a why and an until
                          2.22  Whoever knocks gets locked out
                          2.23  Other people's lists, and countries

Reach further            2.24  One package, four formats
                          2.25  Moving off the firewall already running
                          2.26  Updates arrive on their own
                          2.27  Every rule knows its interface
                          2.28  Outbound traffic
                          2.29  With a keyboard and with a screen reader
                          2.30  Eight languages
                          3.0   Reachable from outside
```

One theme per release, sayable in one sentence — the changelog heading then
writes itself. A model change travels with the feature that justifies it, never
earlier as an end in itself and never twice.

> **Amended after 2.17.** One release was inserted at the head, one was deleted
> from the middle, and one lost a clause. **The numbers in the amendments below
> this one are the numbers as they stood when each was written**; they are a
> record and are not being rewritten. This table is the mapping:
>
> | was | is | | was | is |
> |---|---|---|---|---|
> | 2.18 | 2.19 | | 2.23 | 2.24 |
> | 2.19 | 2.20 | | 2.24 | 2.25 |
> | 2.20 | 2.21 | | 2.25 | 2.26 |
> | 2.21 | 2.22 | | 2.26 | 2.27 |
> | 2.22 | 2.23 | | 2.27 | 2.28 |
> | 2.28 | *deleted* | | 3.1 | *absorbed into 2.18* |
>
> **2.29 and 2.30 keep their numbers.** Deleting one entry cancels inserting
> one exactly, and it is worth stating because a reader who counts will check.
>
> **2.18 — A password alone is not enough** takes the head. easywall has had a
> second factor since 2.8 and it has been a checkbox; the ordering principle is
> exposure, and an administration interface for a firewall reachable over the
> network behind one password is a higher exposure than not being told about a
> rollback.
>
> **2.28 — More than one account is deleted**, not deferred. It is in
> *Deliberately excluded* below with its reasoning. One consequence is worth
> recording: 2.28 was the reason `WriteAuditLog` carries a `user` field that has
> said `web` since it existed. Passkeys give the field a better use — *which
> credential* signed in.
>
> **3.1 — Passkeys is absorbed into 2.18**, and the reason it was 3.1 turned out
> to be half the reason. The entry said WebAuthn needs a registrable domain as
> its Relying Party ID and rejects a bare IP address, which is still true —
> re-checked on 2026-09-10 against WebAuthn Level 3, W3C Candidate
> Recommendation, February 2026, unchanged from the 2020 working-group decision.
> The half nobody had written down: since **Chrome 110**, WebAuthn is refused on
> any origin with a TLS certificate error, and `--ignore-certificate-errors`
> explicitly does not lift it. easywall generates a self-signed certificate by
> default. So the blocker was never only the address — it was the certificate
> too, and that is why **ACME comes out of 3.0 and travels with the passkeys it
> unblocks** rather than waiting for the API. 3.0 keeps its number: it is a
> statement rather than a position, for the reason the 2.12 amendment gives.

> **Amended after 2.16.** Five releases were inserted and a group was added
> above the others, so everything from the old 2.17 down moved. **The numbers in
> the amendments below this one are the numbers as they stood when each was
> written**; they are a record and are not being rewritten. This table is the
> mapping:
>
> | was | is | | was | is |
> |---|---|---|---|---|
> | 2.17 | 2.18 | | 2.22 | 2.25 |
> | 2.18 | 2.20 | | 2.23 | 2.27 |
> | 2.19 | 2.21 | | 2.24 | 2.28 |
> | 2.20 | 2.22 | | 2.25 | 2.30 |
> | 2.21 | 2.23 | | 3.0 / 3.1 | unchanged |
>
> **2.17 — It proves what it says** takes the head of the list, and it takes a
> group of its own. The ordering principle is exposure. A table that reports
> itself enabled while enforcing nothing is a higher exposure than not being told
> about a rollback. The conntrack defect 2.16.0 fixed had shipped for five
> releases, and a stranger on Discord found it. It comes before the notifications
> on a second ground too: a notification is worth what the truth it carries is
> worth.
>
> **2.19 — What it counts can be asked for** is deliberately not folded into
> 2.18. Push and pull reach different consumers, and a scrape format is a
> compatibility promise of its own.
>
> **2.24 and 2.26** are placed by dependency rather than by appetite. An operator
> installs from a package before migrating onto it. The rule model gains the
> interface dimension once, so outbound uses it rather than adding it again.
>
> **2.29 comes before 2.30** because both passes touch every template, and an
> accessibility pass run after eight new locales is an accessibility pass run
> nine times.
>
> **Amended after 2.15.** Two releases were inserted at the head of *Reach
> further* and everything below them moved back two places. Reaching an operator
> who cannot install easywall at all comes before reaching new traffic
> directions or new languages.
>
> **2.21 — One package, four formats** replaces `debian/` rather than adding
> beside it. `release.yml` already rejects a second packaging definition, and
> only a replacement satisfies that reasoning. **Alpine is in on purpose**: it
> buys a second init class, OpenRC, to be tested for as long as it is supported.
>
> **2.22 — Updates arrive on their own** is the first entry here whose cost is
> *operational*. A signing key and a URL outlive any release that produces them,
> which is why it sits after the packaging rather than beside it.
>
> **3.0 and 3.1 keep their numbers**, for the reason the 2.12 amendment gives.

> **Amended in 2.15.** One release was inserted here and everything below it
> moved back one place. **2.16 — The interface looks like a firewall** is not a
> feature; it is catching up on what three design reviews deferred. The
> application sidebar still has the weakness the documentation sidebar lost —
> no divider, no indent, one label/link colour pair for its Dashboard, Rules and
> System groups. `carried-forward.md` already records it as *deliberately
> not touched*. Six identical stat cards with `Manage →` appended is the other
> half, and a typographic position of its own is the third. **3.0 and 3.1 keep
> their numbers**, for the reason the 2.12 amendment already gives: they are
> statements rather than positions.

> **Amended in 2.14.** One release was inserted here and everything below it
> moved back one place. The acceptance window's own visibility turned out to
> need a release of its own, once it gained a deadline and a route to the
> rollback it never had. The counters that would have been 2.14 are 2.15.

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
| **2.18** | **A password alone is not enough** — a second factor becomes a precondition for using the interface, with passkeys as the stronger one and ACME so a browser will accept them | The factor has existed since 2.8 as a checkbox, and `handleFirstRunSkip` exists so it can be declined. Making it mandatory is only defensible if the good version is available, and two things stood in the way of that: WebAuthn rejects a bare IP as its Relying Party ID, and since Chrome 110 it also refuses any origin with a certificate error — which is every default easywall installation. ACME removes the second. The demo is the one exemption |
| **2.14** | **The window shows that it is running** — a countdown that runs, at 40px, on the apply screen and as a chip on every other page, with *Roll back now* beside *Confirm* | For four releases the screen promised the same 120 seconds with a static clock glyph; two screenshots nine seconds apart were pixel-identical, and the rollback it named had no route to the daemon until this release gave it one |
| **2.15** | **You can see it working** — every rule carries a kernel counter and a date, kept across applies | An open port nobody uses is the most common avoidable exposure on a hobby server, and nobody finds it because nobody goes looking |
| **2.17** | **It proves what it says** — a health check for Docker, systemd and monitoring, and a proof that convicts a rule which enforces nothing | For five releases `ct state established,related accept` matched no packet, the invalid-packet drop and the SSH meter reported themselves enabled and enforced nothing, and every surface said the firewall was active. It was found because an operator's VPS went unreachable and they pasted the ruleset into Discord. This is the machinery that would have caught it, at three depths, and the health check the project has never had |
| **2.19** | **When something happens, you hear about it** — a webhook or ntfy push for a rollback, a confirmed apply, panic mode, repeated failed logins | The core still never opens a connection outward; the web process polls the audit log and sends the notification, the same separation as everything else |
| **2.20** | **What it counts can be asked for** — a metrics endpoint | The kernel counters have existed since 2.15 and live only in the interface, so an operator with Grafana cannot see the thing the release was for. Its own entry rather than folded into 2.19, because a scrape format is a second public interface with a compatibility promise — the reasoning that makes 3.0 a major |
| **2.21** | **Every entry has a why and an until** — blacklist entries carry a comment and an expiry | The textarea becomes a table; pasting a list of addresses still works, folded underneath it |
| **2.22** | **Whoever knocks gets locked out** — repeated knocking on closed ports blocks itself, in an nftables set with a timeout, no userspace parser involved | Substitutes for reading `journald`/`auth.log` as root. A named set that fail2ban or CrowdSec can write into covers the credential case without turning the root process into a log parser |
| **2.23** | **Other people's lists, and countries** — curated blocklists and country zones, each switched on individually, all off by default | The web process downloads, never the core. A feed is consulted after the whitelist, unlike your own blacklist — ten thousand entries from someone else's hand should not be able to lock you out of your own address |
| **2.24** | **One package, four formats** — `.deb`, `.rpm`, Arch and Alpine from one description, with `debian/` replaced by an nfpm manifest rather than joined by one | `requirements.md` says of Arch, Fedora and openSUSE that they *should work but are not in CI*: they get a tarball and write the service units themselves. `release.yml` refuses a second packaging definition in as many words — *two definitions of one artefact is how a package comes to contain no binaries* — and only a replacement honours that. The nine paths with their owners and modes are declared once and **proven by four install-verify jobs, not asserted**. Alpine is taken deliberately, knowing it means OpenRC and therefore a second init class to keep tested |
| **2.25** | **Moving off the firewall already running** — read `nft list ruleset` and ufw's rules, and offer them as a staged set | The only entry on this list that removes an *adoption* barrier rather than adding a feature. Whoever already has a firewall copies it out by hand today. After the packaging, because an operator installs first and migrates second |
| **2.26** | **Updates arrive on their own** — a signed APT and RPM repository, so `apt upgrade` and `dnf upgrade` find easywall | The documented install is `wget` and `dpkg -i`, so an operator learns about 2.16 only because the interface says so and installs it by hand. After 2.22, because a repository serves finished packages and not the other way round. It is also this project's first promise that means **operation** rather than code: a signing key to keep for years, and a URL that must not move |
| **2.27** | **Every rule knows its interface** — a rule can name the interface it applies to | The ports page says of itself that every rule *applies to all interfaces*, so a host with a LAN and an uplink cannot express what it means. Before outbound rather than with it: `oifname` is the same dimension, and the rule model should gain it once and be used twice |
| **2.28** | **Outbound traffic** — what the server may send out becomes configurable, `open` (today's behaviour) or `allowlist` | The output chain has policy `ACCEPT` and not one rule today. Highest lockout risk on this list; gets its own acceptance-window round and its own veth proof |
| **2.29** | **With a keyboard and with a screen reader** — one audited pass over every template | `aria-live` appears exactly **once** in the whole interface and `prefers-reduced-motion` three times. Not a feature; catching up, the way 2.16 was. Before the eight languages on purpose — both passes touch every template, and doing this second means re-checking eight locales |
| **2.30** | **Eight languages** — Spanish, Portuguese (BR), Italian, Dutch, Polish, Russian, Chinese (Simplified), Japanese | One pass, once the string set is stable. No RTL: that is a design-system change, not a translation |
| **3.0** | **Reachable from outside** — a REST API with token auth | A major version because an API is a second public interface and a compatibility promise easywall has not made before |

## Deliberately excluded

| | |
|---|---|
| SMTP notifications | Credentials in `web.toml`, foreign mail servers, deliverability — ntfy reaches the same phone without any of it |
| A reimplemented fail2ban | Replaced by the named set the real one can write into (2.22) |
| Zones, on the firewalld model | easywall runs on hosts with one uplink; what zones would be for is covered by `routing.mode` |
| Rule schedules | "Open this port between 08:00 and 18:00" is a state machine nobody can debug once it is in the wrong state |
| IDS/IPS, deep packet inspection, QoS | Different products. easywall filters packets |
| Managing several hosts from one interface | The API in 3.0 makes Ansible possible. A fleet interface is a second product |
| More than one account | The single account is the shape of the product: one operator, one firewall, one audit trail. A second account is a permission model, and a permission model on a box whose whole job is one ruleset is a second product. Was 2.28 until 2.18 |

## What already shipped

Every release, newest first, each with what it was for:
[Changelog]({{ '/docs/changelog/' | relative_url }}).

---

**Next:** [Architecture]({{ '/docs/architecture/' | relative_url }}) ·
[Security]({{ '/docs/security/' | relative_url }}) ·
[Contributing]({{ '/docs/contributing/' | relative_url }})
