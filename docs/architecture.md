---
layout: default
title: Architecture
description: Two processes, a typed socket, three rule sets, and a timer that undoes your mistake.
---

# Architecture

Two processes. The one exposed to the network has no way to touch the firewall.

{% include themed-figure.html base="/assets/diagrams/architecture" ext="svg"
   alt="Browser talks HTTPS to easywall-web, which runs unprivileged; easywall-web talks typed JSON over a Unix socket to easywall-core, which runs as root and speaks netlink to the nftables table inet easywall." %}

## Who may do what

| | `easywall-web` | `easywall-core` |
|---|---|---|
| Runs as | unprivileged user | root, in the `easywall` group, with the capability set cut down to `CAP_NET_ADMIN` |
| Listens on | HTTPS, port 12227 | Unix socket only |
| Can reach nftables | **no** | yes, via netlink |
| Can run a shell | **no** | `nft` only: `--check --file -` to validate a custom rule, `-f -` to apply one |
| Holds | sessions, templates | rule state, acceptance timer |

A flaw in form parsing or template rendering cannot reach the kernel, because the
process holding that flaw has no kernel access to misuse. Reaching the firewall
additionally requires a JSON command the typed protocol accepts and the core's
validation permits.

> **Why it is built this way.** The original easywall — Python, Flask, `iptables`
> via `subprocess` — ran everything as one root process and passed user strings as
> command-line arguments. A web-layer bug was a firewall bug, and arguments could be
> injected. It was archived in 2022 after a CVE. Both root causes are gone here:
> the privileges are in a different process, and every typed rule reaches the
> kernel as a Go struct rather than a command line. Custom rules are the one
> string that still meets `nft`, and what keeps that safe is written out under
> [Security]({{ '/security/' | relative_url }}).

## Three rule sets

{% include themed-figure.html base="/assets/diagrams/rule-states" ext="svg"
   alt="Editing writes to Staged. Applying copies Current to Backup and promotes Staged to Current. If the acceptance window expires, Backup is restored to Current." %}

Editing never touches the kernel. Saving writes to **Staged**; only *Apply* promotes
it to **Current**, and **Backup** is what comes back if you do not confirm.

## Applying, and the way back

{% include themed-figure.html base="/assets/diagrams/apply-flow" ext="svg"
   alt="State machine: editing leads to Staged, applying leads to Live, confirming within the window leads to Confirmed, and letting the window expire leads to Rolled back, from where the staged edits are still available." %}

**Applying is reversible by doing nothing.** If the new rules cut your connection
you cannot click Confirm — and that is exactly what restores the previous set.

| | |
|---|---|
| Window | 120 seconds by default, 10 to 3600 |
| Where the timer lives | the core daemon. Closing the tab or restarting `easywall-web` confirms nothing |
| Stopping the core mid-window | counts as not confirming: the rules roll back before it exits |
| The same idea as | `commit confirmed` on a Cisco router, or the `at now + 5 minutes; iptables -F` experienced operators type before a risky change — here it is the default rather than a habit |

Step by step: [Applying rules]({{ '/features/apply/' | relative_url }}).

## The socket protocol

Fifteen message kinds, declared as Go structs on both sides. Adding an operation
means adding a constant to both ends.

One exception, worth knowing: `SaveRulesPayload.Rules` is an `interface{}` that the
core re-encodes and decodes into the type named by `rule_type`. An unknown
`rule_type` is rejected and the decoded rules are validated before they are stored
— but the field is not typed at the protocol level, and this page used to claim the
whole protocol was.

| Command | Purpose |
|---|---|
| `GET_RULES` · `SAVE_RULES` | read all three sets · write **Staged** |
| `APPLY_RULES` · `ACCEPT` | promote Staged and start the timer · confirm |
| `GET_OPTIONS` · `SAVE_OPTIONS` | protection modules |
| `GET_SETTINGS` · `SAVE_SETTINGS` | IPv6 and Docker |
| `GET_SYSTEM` · `SAVE_SYSTEM` | acceptance window |
| `GET_STATUS` · `GET_LOG` | dashboard · last 200 audit entries |
| `EXPORT_RULES` · `IMPORT_RULES` | backup and restore as JSON |
| `VALIDATE_CUSTOM` | `nft --check` for the live editor |

Full list: [`internal/shared/protocol.go`](https://github.com/jp1337/easywall/blob/main/internal/shared/protocol.go).

## What this does and does not protect you from

| Defends against | How |
|---|---|
| Web-layer vulnerabilities | the web process has no kernel access |
| Argument injection into `nft` | the apply path builds structs, not a command line |
| CSRF | `Origin` and `Sec-Fetch-Site` checked on every unsafe method |
| Locking yourself out | the acceptance window reverts on its own |

| Does **not** defend against | Why |
|---|---|
| A compromised root account | root owns the core |
| A kernel nftables vulnerability | below easywall entirely |
| A legitimate admin making a bad rule | the [audit log]({{ '/features/audit-log/' | relative_url }}) records it, nothing prevents it |

---

**Next:** [Applying rules]({{ '/features/apply/' | relative_url }}) ·
[Configuration]({{ '/configuration/' | relative_url }}) ·
[Security]({{ '/security/' | relative_url }}) ·
[How rules are ordered]({{ '/features/filters/' | relative_url }})
