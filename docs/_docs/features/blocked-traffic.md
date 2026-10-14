---
layout: default
title: Blocked traffic
description: What the firewall refused, newest first — filtered, drilled into, and acted on without leaving the row.
---

# Blocked traffic

The page shows every packet a log switch refused, newest first, and lets you
allowlist, blocklist or open a port from the row that refused it.

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
  <figcaption>Seven columns cover a triage glance (six with the country lookup off); everything else is one click into Details.</figcaption>
</figure>

| Column | Shows |
|---|---|
| Time | When the packet arrived, local time — to the second today, without seconds beyond that; hover for the exact instant |
| Interface | The device it arrived on |
| Source → Destination | Address, and the destination port if there is one |
| Country | The country the source's address block is registered to, as its code; hover for the name, `—` for none. Only with the [country lookup](#where-it-comes-from) on |
| Proto | `tcp`, `udp`, `icmp`, `icmpv6`, or a bare protocol number |
| Rule | Which of the eleven switches refused it, or *Another rule*. A module's name links to its switch on Options; a *Default drop* says why underneath |

**Details** opens the rest: exact time, TCP flags, ICMP type and code, connection state, TTL, source
port, chain and packet mark — whichever of these the packet carried.

## Live

The table asks for new rows every five seconds and shows them on top, marked
with a rule on the left until the next ones arrive. It pauses while you point
at the table or work in it, so no row slides away under the cursor. **Pause**
stops it until you press **Resume**. The line beside the button says whether it
is live and, if not, why. Resuming catches up.

## Where it comes from

The **Country** column is on by default. To switch it off:

```toml
country_lookup = false   # web.toml; or EASYWALL_WEB_COUNTRY_LOOKUP=false
```

Restart `easywall-web`; with it off, no request is made. **An upgraded host
starts fetching once a day unless you switch it off.** `easywall-web` downloads
[ipverse/country-ip-blocks](https://github.com/ipverse/country-ip-blocks) (CC0,
1.75 MB, 238 countries, 248 507 prefixes) from `github.com`, which hands it out
from `release-assets.githubusercontent.com` — no other host — and keeps it in
`<data_dir>/web/countries.tar.gz`. After a failure the old list stays in use
and the download is retried from an hour on. The line under the table says what
is loaded and when it was checked.

| Cost and limits | Measured or enforced |
|---|---|
| Table in memory | about 4 MB; `easywall-web` about +10 MB after a download |
| Lookup per row | about 70 ns |
| Archive / unpacked | at most 8 MiB / 64 MiB |
| One country file | at most 2 MiB |
| Prefixes in all | at most 524 288 |
| Skipped | prefixes broader than IPv4 /4 or IPv6 /16 |

**A registration is not a location.** The column says which country the
address block is registered to. VPNs, cloud regions and Tor exits do not follow
it. Only the web process holds the list; nothing it says reaches the firewall
until you stage a country and apply.

There is no country filter: the filter runs in the core, which has no list,
and filtering the rows on the page (200 by default) would filter a sample.

## Why a default drop refused it

*Default drop* means no switch refused the packet — nothing let it in. The line
under it names what was missing, read from the rules applied **now**:

| The line reads | Because |
|---|---|
| *Port 993/tcp is not open.* | no port rule covers it |
| *993/tcp is open only for 10.0.0.0/8.* | a rule covers it, for other sources |
| *… open only for traffic forwarded to a container* | the rule's scope is `forwarded` |
| *Pings are switched off under Options.* | [Answer pings]({{ '/docs/features/filters/' | relative_url }}#answering-pings) is off, in either family |
| *ICMP type 13 is not accepted.* | a type nothing accepts on its own — see [Always on]({{ '/docs/features/filters/' | relative_url }}#always-on) |
| *… now — this arrived before it was.* | the rules changed since: the port, the allowlist, the blocklist, an ICMP type or the IPv6 mode |

No line appears while the applied rules cannot be read.

## Narrow it down

Six filters sit above the table — source, destination, port, protocol, rule,
interface — and clicking an address, a port or an interface in a row filters to
it directly. The filter is the URL's query string, so the view is exactly what
you bookmark or paste into a message to someone else.

## Act on a row

Four actions per row, only where they make sense — *Open* needs a destination
port and a protocol the firewall can open:

| Action | Does |
|---|---|
| **Allowlist** | Stages the source address on the allowlist |
| **Blocklist** | Stages the source address on the blocklist |
| **Open *port*** | Stages a port rule for the packet's destination port and protocol |
| **Block *XX*** | Stages the source's country, as [Block a country]({{ '/docs/features/countries/' | relative_url }}) does, at most 30. Only for a country not on yet, never on a forwarded packet: countries filter what reaches this host |

| The packet was refused by | Offered | Why not the others |
|---|---|---|
| Default drop | allowlist · blocklist · open the port | — |
| Bogon filter | allowlist · blocklist | a port rule runs after the filter |
| Another rule (your custom rules) | allowlist · blocklist | opening the port would override a rule you wrote |
| The blocklist | a link to the blocklist | the blocklist is checked before the allowlist |
| A protection module (SSH, SYN, ICMP, RST, port scan, invalid, fragment) | blocklist | modules run before the allowlist and the ports |

The table pauses while you point at it or work in it, and catches up when you leave.

**They stage. They never apply.** A staged count appears beside the page title
and links to [Apply]({{ '/docs/features/apply/' | relative_url }}); nothing
reaches the kernel, and no acceptance window starts, until you go there.

> **It will not let you lock yourself out.** Blocklisting the address you are
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

Accepted traffic, bytes per session, a location and reverse DNS. It is a list
of packets the firewall refused, decoded from what NFLOG hands the core. The
country column is the one exception, and it is a registration looked up in the
list this host downloaded — never a request per address.
