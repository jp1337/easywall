---
layout: default
title: Roadmap
description: What is planned, in which order, and why that order — correctness before reach.
---

# Roadmap

Correctness first: a firewall that quietly does less than it says is worse than
one that does less and says so.

| Version | What | Why it comes when it does |
|---|---|---|
| **2.7** | **Proof, not counts** — tests that assert *meaning*, then real packets through a veth pair | The suite asserts rule counts today. A rule that dropped where it should accept would pass it |
| **2.8** | **Identity** — a `user` field in the protocol, then login events, then multiple accounts, then 2FA | Every audit entry says `web` and logins are not recorded at all. The field must exist before login events mean anything, and a second factor on a single account is not worth much |
| **2.9** | **Reach** — a REST API with token authentication for Ansible and scripting; Let's Encrypt/ACME as a strictly optional alternative to a reverse proxy | Built on the accounts from 2.8. Running with no outbound connection stays the default |
| **2.9** | **Trusted reverse proxy**, opt-in — a list of proxy addresses whose `X-Forwarded-For` is believed | Behind a proxy today, every request looks like it comes from the proxy, so the login rate limit is shared by everyone. It must be a list of addresses and never a boolean: "trust the header" is the vulnerability, not the feature |
| **2.10** | **Knowing how many machines this runs on** — an opt-in count | A critical bug matters differently at ten installations and at ten thousand, and right now nobody knows which this is |

## On counting installations

Opt-in and not opt-out, because this site and [Security]({{ '/docs/security/' | relative_url }})
promise that the update check is the *only* outbound request, and because an
administrative interface quietly reporting to its author is the thing easywall
removed Google Fonts to avoid. A security tool that has to explain a surprising
connection has already lost the argument.

What it sends is printed verbatim in
[Configuration]({{ '/docs/configuration/' | relative_url }}#counting-installations): a
random identifier generated on the machine, and the version. Enough to count
distinct installations and to say "the fix reached 80% of them", and not enough to
describe anyone.

Worth checking first: the update check already reaches `api.github.com` daily from
every installation that has not disabled it, and release assets record their own
download counts. That is a usable lower bound today, for nothing.

## Done in 2.6

| | |
|---|---|
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
