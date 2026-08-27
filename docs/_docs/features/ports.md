---
layout: default
title: Port Rules
description: Which ports accept inbound connections, and why marking your SSH port matters.
---

# Port Rules

Ports that accept inbound connections on every interface. TCP and UDP are separate
tabs; a rule is a port, an optional SSH mark, and a description for your own benefit.

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/ports" ext="png"
     alt="The port rules page: TCP and UDP tabs, a filter box, a table of ports with an SSH protection checkbox and description, and context cards explaining port syntax and SSH protection." %}
  <figcaption>Filtering narrows what is already on the page, so unsaved edits survive it.</figcaption>
</figure>

## What you can write

| Form | Example | Opens |
|---|---|---|
| Single port | `443` | one port |
| Range | `8000:9000` | 8000 to 9000, inclusive |

1–65535, ranges ascending. Every rule applies to all interfaces — to restrict by
source, use the [whitelist]({{ '/docs/features/blacklist/' | relative_url }}) or a
[custom rule]({{ '/docs/features/custom-rules/' | relative_url }}).

## Mark your SSH port

Ticking **SSH protection** routes that port through the brute-force chain, which
rate-limits new connections per source address.

> **Mark it even on a non-standard port.** The chain is applied per port, so 2222 is
> protected exactly like 22 — but only if you tick it. If you mark nothing, easywall
> meters 22 by default, which is the wrong port on a hardened host.
>
> Metering is not opening. Since 2.11 the chain hands the packet back to the rest
> of the input chain instead of accepting it, so a port is open only if a rule
> opens it. Before 2.11 a marked-nothing host had 22 accepted by the module alone.

Each source address gets its own budget, so somebody else being rate-limited does not
affect you. A [whitelist]({{ '/docs/features/blacklist/' | relative_url }}) entry does not
exempt you from it, though — protection modules are consulted before the whitelist,
as the [rule order]({{ '/docs/features/filters/' | relative_url }}) shows.

The mark alone does nothing unless the module is switched on under
[options]({{ '/docs/features/filters/' | relative_url }}) — it is on by default, limit 5.

## Common sets

| Server | TCP | UDP |
|---|---|---|
| Web | `80`, `443` | — |
| Mail | `25`, `465`, `587`, `993`, `995` | — |
| DNS | `53` | `53` |
| WireGuard | — | `51820` |
| Minecraft | `25565` (Java) | `25565` (Bedrock) |
| SSH, moved | `2222` ✓ SSH protection | — |

## Nothing happens until you apply

Saving stages. Deleting stages too — the rule keeps working until you
[apply]({{ '/docs/features/apply/' | relative_url }}).

## When it does not work

| Symptom | Cause | Check |
|---|---|---|
| Port open, connection refused | Nothing is listening | `ss -tlnp \| grep <port>` |
| Port listed, still blocked | Not applied yet | Go to **Apply rules** |
| Blocked despite being open | The source is on the [blacklist]({{ '/docs/features/blacklist/' | relative_url }}), which is checked first | |
| SSH drops right after Apply | That is the design — do nothing and the old rules come back | |
| Your own SSH is rate-limited | You hit your own brute-force budget | Wait a minute, or raise `ssh_brute_force_connection_limit`. The whitelist does **not** help: modules run before it |
