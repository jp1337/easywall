---
layout: default
title: Blocked traffic
description: What the firewall refused, newest first — filtered, drilled into, and acted on without leaving the row.
---

# Blocked traffic

The page shows every packet a log switch refused, newest first, and lets you
whitelist, blacklist or open a port from the row that refused it.

## Turn logging on

Every switch is off by default — nothing appears here until at least one is on.
Open [**Options → Logging**]({{ '/docs/features/filters/' | relative_url }}#logging)
and switch one on. The two worth starting with:

| Switch | Logs |
|---|---|
| `log_blocked_connections` | Everything the final policy drops — the broadest one |
| `ssh_brute_force_log` | Connections over the SSH rate — the one most hosts get hit with first |

## Read a row

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/blocked" ext="png"
     alt="The Blocked traffic page: a filter bar above a table of refused packets, each row carrying the actions its rule allows and a details disclosure." %}
  <figcaption>Six columns cover a triage glance; everything else is one click into Details.</figcaption>
</figure>

| Column | Shows |
|---|---|
| Time | When the packet arrived, local time — to the second today, without seconds beyond that; hover for the exact instant |
| Interface | The device it arrived on |
| Source → Destination | Address, and the destination port if there is one |
| Proto | `tcp`, `udp`, `icmp`, `icmpv6`, or a bare protocol number |
| Rule | Which of the ten switches refused it, or *Another rule* |

**Details** opens the rest: exact time, TCP flags, connection state, TTL, source
port, chain and packet mark — whichever of these the packet carried.

## Narrow it down

Six filters sit above the table — source, destination, port, protocol, rule,
interface — and clicking an address, a port or an interface in a row filters to
it directly. The filter is the URL's query string, so the view is exactly what
you bookmark or paste into a message to someone else.

## Act on a row

Three actions per row, only where they make sense — *Open* needs a destination
port and a protocol the firewall can open:

| Action | Does |
|---|---|
| **Whitelist** | Stages the source address on the whitelist |
| **Blacklist** | Stages the source address on the blacklist |
| **Open *port*** | Stages a port rule for the packet's destination port and protocol |

| The packet was refused by | Offered | Why not the others |
|---|---|---|
| Default drop | whitelist · blacklist · open the port | — |
| Bogon filter | whitelist · blacklist | a port rule runs after the filter |
| Another rule (your custom rules) | whitelist · blacklist | opening the port would override a rule you wrote |
| The blacklist | a link to the blacklist | the blacklist is checked before the whitelist |
| A protection module (SSH, SYN, ICMP, RST, port scan, invalid, fragment) | blacklist | modules run before the whitelist and the ports |

The table pauses while you point at it or work in it, and catches up when you leave.

**They stage. They never apply.** A staged count appears beside the page title
and links to [Apply]({{ '/docs/features/apply/' | relative_url }}); nothing
reaches the kernel, and no acceptance window starts, until you go there.

> **It will not let you lock yourself out.** Blacklisting the address you are
> signed in from — or, behind a reverse proxy, the proxy's address — is refused
> with the reason, before anything is staged. The same check the apply screen
> runs.

## When the page is empty

| State | Reads |
|---|---|
| The core could not be read | *Core daemon unreachable:* and the error |
| Nothing switched on | *Nothing is being logged* — with a link to the switches |
| On, nothing refused since *time* | *Nothing has been refused since …* |
| Not running, never bound | a banner naming the NFLOG group and why the bind failed; logged packets go to the kernel log instead |
| Not running, stopped after running | a banner saying so; nothing is logged anywhere — not here, not the kernel log — until `easywall-core` restarts |

## What it does not show

Accepted traffic, bytes per session, geolocation and reverse DNS. It is a list
of packets the firewall refused, decoded from what NFLOG hands the core — nothing
this page shows was looked up anywhere else.
