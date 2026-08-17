---
layout: default
title: Firewall Filters
description: The protection modules — what each one drops, how to tune it, and which are on by default.
---

# Firewall Filters

Optional modules that harden the host beyond opening and closing ports. All of them
are native nftables rules in `table inet easywall` — no subprocess, nothing to inject
into. Toggling one is staged like any other change and takes effect on
[Apply]({{ '/docs/features/apply/' | relative_url }}).

## Which to turn on

The question most people arrive with. Details for each module are below.

| Host | Turn on | Leave off |
|---|---|---|
| Public server, static address | Everything under Attack protection, plus the bogon filter | Fragment drop, unless you know your traffic |
| Behind NAT, or on a LAN | SSH brute-force, SYN flood, port scan, invalid packets. The bogon filter too, once your own network is whitelisted | Broadcast/multicast/anycast |
| Container host | The defaults. The bogon filter is safe with Docker coexistence on — bridge networks are exempt | — |

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/options" ext="png"
     alt="The firewall options page: a grid of protection module cards under Attack protection and Traffic filtering, each with a toggle and its own parameters. An accent edge marks a module that is switched on." %}
  <figcaption>An accent edge down the left of a card means that module is on.</figcaption>
</figure>

## Where the modules sit

Before the blacklist, before the whitelist, before any port is considered. A module
that drops a packet drops it whatever else you have allowed.

Two things come earlier still: loopback, always, and the [IPv6
mode]({{ '/docs/features/system-settings/' | relative_url }}) — set to `passthrough` or
`block`, IPv6 is decided before any module sees it.

{% include themed-figure.html base="/assets/diagrams/rule-order" ext="svg"
   alt="Decision flow for an incoming packet: loopback first, then the IPv6 mode, which accepts or drops all IPv6 outright unless it is set to filter; then established connections and ICMP, then protection modules, then Docker bridge networks, then the blacklist which drops, then the whitelist which accepts every port, then open ports, then custom rules, and finally the chain policy which drops." %}

## Always on

Compiled into every rule set. There is no switch for these.

| Rule | nftables | Why |
|---|---|---|
| Default DROP | `policy drop` on `input` | Deny by default |
| Loopback | `iif lo accept` | Local processes must reach each other |
| Return traffic | `ct state {related, established} accept` | Replies to what you started |
| ICMPv4 | types 0, 3, 11, 12 | Echo reply, unreachable, TTL exceeded, parameter problem |
| ICMPv6 | types 1–4, 128, 129 | The minimum IPv6 needs to work at all |
| ICMPv6 discovery | types 133–136, when enabled | Address autoconfiguration — see [network settings]({{ '/docs/features/system-settings/' | relative_url }}) |

## The three chains

Everything above is the `input` chain — traffic addressed to this host. easywall
creates two more, and what they do has consequences worth knowing.

| Chain | Policy | What reaches it |
|---|---|---|
| `input` | **drop** | Traffic addressed to this host. Everything on this page. |
| `output` | **accept** | Traffic this host sends. easywall does not filter it. |
| `forward` | **drop** by default | Traffic this host would *route* — between two interfaces, out of a container, into a published container port. Governed by `[routing]` |

> **Closed is not the same as unfiltered.** A base chain whose rules give no verdict
> falls through to its policy, so an empty `forward` chain with `policy drop` destroys
> every routed packet at that hook — including ones another table has already
> accepted. The drop is final: a forward chain of your own cannot overrule it, and
> [custom rules]({{ '/docs/features/custom-rules/' | relative_url }}) go into `input`.
> Costs nothing on a plain server; stopped every Docker container dead until 2.5.0.

Two things cross that chain:

- The [Docker]({{ '/docs/features/docker/' | relative_url }}) networks you have allowed,
  whatever `routing.mode` says. Switching coexistence on is already the statement
  that this host carries container traffic.
- Whatever `[routing]` names. Its three positions — route nothing, route these
  networks, leave routed traffic alone — are on the
  [Network page]({{ '/docs/features/system-settings/' | relative_url }}#the-network-page)
  and in [configuration]({{ '/docs/configuration/' | relative_url }}#routing).

## Attack protection

| Module | Drops | Tuning | Default |
|---|---|---|---|
| **SSH brute-force** | New SSH connections from one source above its rate. Applies to ports marked *SSH protection* on the [ports page]({{ '/docs/features/ports/' | relative_url }}), and to 22 if none is marked | `ssh_brute_force_connection_limit` — 5/min | **on** |
| **ICMP flood** | Echo requests from one source above its rate — ICMP type 8 and ICMPv6 type 128 | `icmp_flood_connection_limit` — 10/s | **on** |
| **SYN flood** | New TCP connections from one source above its rate | `syn_flood_limit` — 100/s | **on** |
| **Port scan detection** | Seven impossible TCP flag combinations: NULL, FIN alone, SYN+FIN, RST+FIN, SYN+RST, XMAS and all-flags — none of which a real client sends | — | **on** |
| **Invalid packets** | Packets conntrack cannot match to a connection | — | **on** |
| **Fragment drop** | Fragmented **IPv4** packets | — | off |
| **Bogon filter** | Impossible **IPv4** source addresses on a non-loopback interface | — | off |
| **Connection limit** | Simultaneous connections from one source above its cap | `connection_limit_max` — 100 | off |
| **TCP RST flood** | Inbound RST packets from one source above its rate | `tcp_rst_flood_limit` — 100/s | off |

> **Every rate is counted per source address** — one kernel counter per address, in a
> set whose entries expire when that source goes quiet. A flood from one host cannot
> spend the budget that keeps you connected.
>
> Not true before 2.5.0: four modules held a single counter for the whole machine, so
> five SSH attempts a minute from anywhere locked out the administrator too.

### What the bogon filter drops

A packet claiming to come from one of these, arriving on a real interface, is
spoofed — nothing on the public internet legitimately has such a source address.

| Range | | Range | |
|---|---|---|---|
| `0.0.0.0/8` | "this network" | `172.16.0.0/12` | private |
| `10.0.0.0/8` | private | `192.0.2.0/24` | documentation |
| `100.64.0.0/10` | carrier NAT | `192.168.0.0/16` | private |
| `127.0.0.0/8` | loopback | `198.51.100.0/24` | documentation |
| `169.254.0.0/16` | link-local | `203.0.113.0/24` | documentation |
| | | `240.0.0.0/4` | reserved |

**IPv4 only.** There is no IPv6 counterpart, and it would not be a translation of
this one: `fe80::/10` is link-local, and IPv6 needs neighbour discovery on it to
function at all.

### What it does not drop

Anything on the whitelist, and any Docker bridge network, is exempt. Both are lists
of RFC 1918 addresses, which is exactly what this filter drops — and it runs before
either of them, so switching it on used to turn both features off without saying so.
Whitelisting `192.168.1.0/24` had no effect at all, and neither did letting Docker's
`172.17.0.0/16` through.

An exemption is narrow: it covers what you allowed and nothing more. Whitelist
`192.168.1.0/24` and the rest of `192.168.0.0/16` is still dropped.

```
# nft list chain inet easywall bogon
ip saddr 192.168.1.0/24 return      ← whitelisted
ip saddr 172.17.0.0/16 return       ← Docker bridge
ip saddr 10.0.0.0/8 drop
ip saddr 192.168.0.0/16 drop        ← the rest of the range, still dropped
...
```

| Before switching it on | Why |
|---|---|
| Whitelist the address you administer from, if it is RFC 1918 | the exemption only covers what is on the list when the rules are applied |
| Not on a DHCP server | a client requesting a lease has no address yet and sends from `0.0.0.0`, which this filter drops |

## Traffic filtering

| Module | Drops | Default |
|---|---|---|
| **Drop broadcast** | Traffic to a broadcast address | off |
| **Drop multicast** | Traffic to a multicast group | off |
| **Drop anycast** | Traffic to an anycast destination | off |

> **Not on a LAN.** These carry DHCP, mDNS and IPv6 neighbour discovery. Safe to drop
> on a public-facing host with a static address; disruptive nearly everywhere else.

## Logging

Every module has its own `*_log` switch, plus two global ones. All of it is
rate-limited, which is what the `*_limit` values in messages per minute are for — a
flood must not be able to fill the disk.

| Switch | Logs | Prefix |
|---|---|---|
| `ssh_brute_force_log` | Connections over the SSH rate | `easywall ssh:` |
| `icmp_flood_log` | Echo requests over the rate | `easywall icmp-flood:` |
| `syn_flood_log` | SYNs over the rate | `easywall syn-flood:` |
| `tcp_rst_flood_log` | RSTs over the rate | `easywall tcp-rst:` |
| `port_scan_log` | Scan flag combinations | `easywall portscan:` |
| `drop_invalid_packets_log` | Packets in INVALID state | `easywall invalid:` |
| `drop_fragments_log` | Fragmented packets | `easywall fragment:` |
| `bogon_filter_log` | Bogon sources | `easywall bogon:` |
| `log_blacklist_connections` | Blacklist hits, before the drop | `easywall blacklist:` |
| `log_blocked_connections` | Everything the final policy drops | `easywall drop:` |

```bash
journalctl -k -f | grep easywall
```

Each log rule sits directly in front of the drop it belongs to and carries the
same match, so what appears in the log is exactly what was dropped.

> **None of this worked before 2.5.0.** Eight switches produced no rule at all, and
> the one that did carried no prefix — the log expression's `Key` field is a bitmask
> and was set to a bare attribute number, so the kernel got an empty log group. The
> command above matched nothing, whatever was switched on.

This is the *kernel* log — packets. Administrative changes are in the
[audit log]({{ '/docs/features/audit-log/' | relative_url }}) instead.

The interface writes the same `[firewall]` section you would edit by hand; every key
is listed under [Configuration]({{ '/docs/configuration/' | relative_url }}#firewall--protection-modules).
