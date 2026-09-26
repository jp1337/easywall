---
layout: default
title: Roadmap
description: Fourteen releases, ordered by exposure — what it quietly does not do first, then maintenance, then reach.
---

# Roadmap

Correctness first: a firewall that quietly does less than it says is worse than
one that does less and says so. What follows is planned in this order, not
promised in it, and corrected when something changes. What already shipped is in
the [Changelog]({{ '/docs/changelog/' | relative_url }}).

**Ordering principle: by exposure.** A gap in what the firewall enforces comes
first, then what lets you maintain it, then what reaches further:

```
Say what it does         2.24  Containers answer over IPv6
                         2.25  What it counts can be asked for

Be able to maintain it   2.26  Every entry has a why and an until
                         2.27  Whoever knocks gets locked out
                         2.28  Countries

Reach further            2.29  One package, four formats
                         2.30  Moving off the firewall already running
                         2.31  Updates arrive on their own
                         2.32  Every rule knows its interface
                         2.33  Outbound traffic
                         2.34  With a keyboard and with a screen reader
                         2.35  Eight languages
                         2.36  A failed passkey says which origin it expected
                         3.0   Reachable from outside
```

One theme per release, sayable in one sentence — the changelog heading then
writes itself. A model change travels with the feature that justifies it, never
earlier as an end in itself and never twice.

| Version | What | Why it comes when it does |
|---|---|---|
| **2.24** | **Containers answer over IPv6** — a port Docker publishes on IPv6 is forwarded like its IPv4 twin, and each bridge's IPv6 subnets are detected | Every forwarded rule is IPv4 today. An IPv6 client of a container is dropped, and nothing says so. A production host running mail over IPv6 through Docker waits on it |
| **2.25** | **What it counts can be asked for** — a metrics endpoint | The kernel counters have existed since 2.15 and live only in the interface, so an operator with Grafana cannot see the thing the release was for. Its own entry rather than folded into 2.20, because a scrape format is a second public interface with a compatibility promise — the reasoning that makes 3.0 a major. `DESIGN.md`'s Known Gaps names the missing data-visualisation language; this release either brings one or states plainly that it does not, rather than that decision arriving mid-build |
| **2.26** | **Every entry has a why and an until** — blocklist entries carry a comment and an expiry | The textarea becomes a table; pasting a list of addresses still works, folded underneath it |
| **2.27** | **Whoever knocks gets locked out** — repeated knocking on closed ports blocks itself, in an nftables set with a timeout, no userspace parser involved | Substitutes for reading `journald`/`auth.log` as root. A named set that fail2ban or CrowdSec can write into covers the credential case without turning the root process into a log parser — and the CrowdSec bouncer's `set-only` mode, which 2.23 deliberately does not document. **Closed ports answer**: the default policy gains `reject` — a TCP reset, ICMP port-unreachable otherwise, rate-limited, dropping above the limit — beside today's silent `drop`. A silent drop makes a misconfiguration look like a hang: a 2.23 production host with an AAAA record and IPv4-only mail ports left mail clients waiting for a timeout instead of falling back to IPv4. Stealth buys little on a host that answers on its open ports. The same release, because it decides what a closed port tells whoever knocks on it |
| **2.28** | **Countries** — country zones, each switched on individually, all off by default | Split off from the feeds, which shipped as 2.23. Built from the RIRs' delegation files, which need no account; the web process downloads, never the core |
| **2.29** | **One package, four formats** — `.deb`, `.rpm`, Arch and Alpine from one description, with `debian/` replaced by an nfpm manifest rather than joined by one | `requirements.md` says of Arch, Fedora and openSUSE that they *should work but are not in CI*: they get a tarball and write the service units themselves. `release.yml` refuses a second packaging definition in as many words — *two definitions of one artefact is how a package comes to contain no binaries* — and only a replacement honours that. The nine paths with their owners and modes are declared once and **proven by four install-verify jobs, not asserted**. Alpine is taken deliberately, knowing it means OpenRC and therefore a second init class to keep tested |
| **2.30** | **Moving off the firewall already running** — read `nft list ruleset` and ufw's rules, and offer them as a staged set | The only entry on this list that removes an *adoption* barrier rather than adding a feature. Whoever already has a firewall copies it out by hand today. After the packaging, because an operator installs first and migrates second |
| **2.31** | **Updates arrive on their own** — a signed APT and RPM repository, so `apt upgrade` and `dnf upgrade` find easywall | The documented install is `wget` and `dpkg -i`, so an operator learns about 2.16 only because the interface says so and installs it by hand. After 2.29, because a repository serves finished packages and not the other way round. It is also this project's first promise that means **operation** rather than code: a signing key to keep for years, and a URL that must not move |
| **2.32** | **Every rule knows its interface** — a rule can name the interface it applies to | The ports page says of itself that every rule *applies to all interfaces*, so a host with a LAN and an uplink cannot express what it means. Before outbound rather than with it: `oifname` is the same dimension, and the rule model should gain it once and be used twice. **`detectDockerBridges` collects IPv4 addresses only**, so the bridge exception and 2.19's deny never match an IPv6 packet and a Docker network with IPv6 has to be named in `docker.custom_networks` by hand. 2.20.1's published-port warning inherits the same limit from the same place: it reads Docker's DNAT rules in the `ip` family, so an IPv6 published port is never seen and never named — and silence there is not a clearance. It lands here because it is the same dimension — which interface a rule is about — and because it is not a fix that rides along: teaching detection to return IPv6 CIDRs starts accepting traffic that is dropped today, on hosts nobody has measured, so it needs its own acceptance round and its own integration test against a real IPv6-enabled bridge. Spec §5 of `docs-tech/specs/2026-09-12-2.19-what-it-passes-on-it-also-filters.md` is the write-up |
| **2.33** | **Outbound traffic** — what the server may send out becomes configurable, `open` (today's behaviour) or `allowlist` | The output chain has policy `ACCEPT` and not one rule today. Highest lockout risk on this list; gets its own acceptance-window round and its own veth proof |
| **2.34** | **With a keyboard and with a screen reader** — one audited pass over every template | `aria-live` appears exactly **once** in the whole interface and `prefers-reduced-motion` three times. Not a feature; catching up, the way 2.16 was. Before the eight languages on purpose — both passes touch every template, and doing this second means re-checking eight locales |
| **2.35** | **Eight languages** — Spanish, Portuguese (BR), Italian, Dutch, Polish, Russian, Chinese (Simplified), Japanese | One pass, once the string set is stable. No RTL: that is a design-system change, not a translation. `fr` sits at 471 of 620 keys (76.0%) while `en` and `de` hold exact parity; finishing it is part of this pass, not a separate errand |
| **2.36** | **A failed passkey says which origin it expected** — the ceremony names the mismatch instead of logging "did not verify" | `publicOrigin()` builds the expected origin from this process's own port, so a browser arriving through a terminating reverse proxy reports the proxy's origin and every assertion is refused. [Reverse proxy]({{ '/docs/installation/reverse-proxy/' | relative_url }}) already rules that passkeys do not work through one, and the refusal is correct — but `passkeyUnavailableReason` reaches it by asking about the **certificate**, and an operator who gives the process a real certificate behind the proxy clears that reason and enables a button that cannot succeed. Both origins are known at `ValidateLogin`, so the failure can name itself. Reported from a traefik deployment where the backend certificate was about to be fixed for unrelated and good reasons. It is not a knob: an operator assertion that a proxy terminates in front would disable the certificate check and leave the origin untouched — a safety check switched off in exchange for nothing |
| **3.0** | **Reachable from outside** — a REST API with token auth | A major version because an API is a second public interface and a compatibility promise easywall has not made before |

**Deferred: a list published from root01xvp.** One sensor behind the Hetzner
firewall sees SSH brute force only, which blocklist.de and DShield already
publish. Publishing addresses needs a GDPR balancing test, a delisting channel
and an Art. 30 record.

**Deferred: a refresh that would drop your own address.** A refresh loads with
no acceptance window, so a later copy listing the address you manage the host
from locks you out with no rollback. A guard that refuses such a copy needs a
spec amendment and a later release. Until then, allowlist that address.

## Deliberately excluded

| | |
|---|---|
| SMTP notifications | Credentials in `web.toml`, foreign mail servers, deliverability — ntfy reaches the same phone without any of it |
| A reimplemented fail2ban | Replaced by the named set the real one can write into (2.27) |
| Zones, on the firewalld model | easywall runs on hosts with one uplink; what zones would be for is covered by `routing.mode` |
| Rule schedules | "Open this port between 08:00 and 18:00" is a state machine nobody can debug once it is in the wrong state |
| IDS/IPS, deep packet inspection, QoS | Different products. easywall filters packets |
| Managing several hosts from one interface | The API in 3.0 makes Ansible possible. A fleet interface is a second product |
| More than one account | The single account is the shape of the product: one operator, one firewall, one audit trail. A second account is a permission model, and a permission model on a box whose whole job is one ruleset is a second product. |

## What already shipped

Every release, newest first, each with what it was for:
[Changelog]({{ '/docs/changelog/' | relative_url }}).

---

**Next:** [Architecture]({{ '/docs/architecture/' | relative_url }}) ·
[Security]({{ '/docs/security/' | relative_url }}) ·
[Contributing]({{ '/docs/contributing/' | relative_url }})
