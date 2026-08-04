---
layout: default
title: Architecture
description: Two processes, a typed socket, three rule sets, and a timer that undoes your mistake.
---

# Architecture

Two processes. The one exposed to the network has no way to touch the firewall.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="{{ '/assets/diagrams/architecture-dark.svg' | relative_url }}">
  <img src="{{ '/assets/diagrams/architecture-light.svg' | relative_url }}" alt="Browser talks HTTPS to easywall-web, which runs unprivileged; easywall-web talks typed JSON over a Unix socket to easywall-core, which runs as root and speaks netlink to the nftables table inet easywall.">
</picture>

## Who may do what

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="{{ '/assets/diagrams/privilege-split-dark.svg' | relative_url }}">
  <img src="{{ '/assets/diagrams/privilege-split-light.svg' | relative_url }}" alt="easywall-web handles TLS, sessions and templates as an unprivileged user. easywall-core holds the rule state, the acceptance timer and the netlink connection as root. Only typed JSON commands cross between them.">
</picture>

| | `easywall-web` | `easywall-core` |
|---|---|---|
| Runs as | unprivileged user | root or `CAP_NET_ADMIN` |
| Listens on | HTTPS, port 12227 | Unix socket only |
| Can reach nftables | **no** | yes, via netlink |
| Can run a shell | **no** | only `nft -f -` for custom rules |
| Holds | sessions, templates | rule state, acceptance timer |

A flaw in form parsing or template rendering cannot reach the kernel, because the
process holding that flaw has no kernel access to misuse. Reaching the firewall
additionally requires a JSON command the typed protocol accepts and the core's
validation permits.

> **Why it is built this way.** The original easywall — Python, Flask, `iptables`
> via `subprocess` — ran everything as one root process and passed user strings as
> command-line arguments. A web-layer bug was a firewall bug, and arguments could be
> injected. It was archived in 2022 after a CVE. Both root causes are gone here:
> the privileges are in a different process, and the apply path builds Go structs
> instead of a command line.

## Three rule sets

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="{{ '/assets/diagrams/rule-states-dark.svg' | relative_url }}">
  <img src="{{ '/assets/diagrams/rule-states-light.svg' | relative_url }}" alt="Editing writes to Staged. Applying copies Current to Backup and promotes Staged to Current. If the acceptance window expires, Backup is restored to Current.">
</picture>

Editing never touches the kernel. Saving writes to **Staged**; only *Apply* promotes
it to **Current**, and **Backup** is what comes back if you do not confirm.

## Applying, and the way back

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="{{ '/assets/diagrams/apply-flow-dark.svg' | relative_url }}">
  <img src="{{ '/assets/diagrams/apply-flow-light.svg' | relative_url }}" alt="State machine: editing leads to Staged, applying leads to Live, confirming within the window leads to Confirmed, and letting the window expire leads to Rolled back, from where the staged edits are still available.">
</picture>

**Applying is reversible by doing nothing.** If the new rules cut your connection,
you cannot click Confirm — and that is exactly what restores the previous set.
Default window: 120 seconds, configurable from 10 to 3600.

The same idea as `commit confirmed` on a Cisco router, or the `at now + 5 minutes;
iptables -F` that experienced operators type before a risky change. Here it is the
default path rather than a habit you have to remember.

## The socket protocol

About 16 typed message kinds, declared as Go structs on both sides. No untyped
fields, no `map[string]interface{}` pass-through — adding an operation means adding
a constant to both ends.

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

**Next:** [Configuration]({{ '/configuration/' | relative_url }}) · [Security]({{ '/security/' | relative_url }}) · [How rules are ordered]({{ '/features/filters/' | relative_url }})
