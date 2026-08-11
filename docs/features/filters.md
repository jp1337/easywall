---
layout: default
title: Firewall Filters
description: The protection modules — what each one drops, how to tune it, and which are on by default.
---

# Firewall Filters

Optional modules that harden the host beyond opening and closing ports. All of them
are native nftables rules in `table inet easywall` — no subprocess, nothing to inject
into. Toggling one is staged like any other change and takes effect on **Apply**.

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/options" ext="png"
     alt="The firewall options page: a grid of protection module cards under Attack protection and Traffic filtering, each with a toggle and its own parameters. An accent edge marks a module that is switched on." %}
  <figcaption>An accent edge down the left of a card means that module is on.</figcaption>
</figure>

## Where the modules sit

Before the blacklist, before the whitelist, before any port is considered. A module
that drops a packet drops it whatever else you have allowed.

Two things come earlier still: loopback, always, and the [IPv6
mode]({{ '/features/system-settings/' | relative_url }}) — set to `passthrough` or
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
| ICMPv6 discovery | types 133–136, when enabled | Address autoconfiguration — see [network settings]({{ '/features/system-settings/' | relative_url }}) |

## The three chains

Everything above is the `input` chain — traffic addressed to this host. easywall
creates two more, and what they do has consequences worth knowing.

| Chain | Policy | What reaches it |
|---|---|---|
| `input` | **drop** | Traffic addressed to this host. Everything on this page. |
| `output` | **accept** | Traffic this host sends. easywall does not filter it. |
| `forward` | **drop** by default | Traffic this host would *route* — between two interfaces, out of a container, into a published container port. Governed by `[routing]` |

> **`forward` is closed by default, and that is not the same as unfiltered.** A base
> chain whose rules give no verdict falls through to its policy, so an empty chain
> with `policy drop` drops everything at that hook — including packets another
> table's forward chain has already accepted. A drop here is final: a forward chain
> of your own in another table cannot overrule it, and
> [custom rules]({{ '/features/custom-rules/' | relative_url }}) are appended to
> `input`. On a plain server nothing is routed and this costs nothing. On a host
> that routes — a container host, a VPN gateway — it stops the traffic dead, which
> is what it did to every Docker container until 2.5.0.

Two things cross that chain:

- The [Docker]({{ '/features/docker/' | relative_url }}) networks you have allowed,
  whatever `routing.mode` says. Switching coexistence on is already the statement
  that this host carries container traffic.
- Whatever `[routing]` names. Its three positions — route nothing, route these
  networks, leave routed traffic alone — are on the
  [Network page]({{ '/features/system-settings/' | relative_url }}#the-network-page)
  and in [configuration]({{ '/configuration/' | relative_url }}#routing).

## Attack protection

| Module | Drops | Tuning | Default |
|---|---|---|---|
| **SSH brute-force** | New SSH connections from one source above its rate. Applies to ports marked *SSH protection* on the [ports page]({{ '/features/ports/' | relative_url }}), and to 22 if none is marked | `ssh_brute_force_connection_limit` — 5/min | **on** |
| **ICMP flood** | Echo requests from one source above its rate — ICMP type 8 and ICMPv6 type 128 | `icmp_flood_connection_limit` — 10/s | **on** |
| **SYN flood** | New TCP connections from one source above its rate | `syn_flood_limit` — 100/s | **on** |
| **Port scan detection** | Seven impossible TCP flag combinations: NULL, FIN alone, SYN+FIN, RST+FIN, SYN+RST, XMAS and all-flags — none of which a real client sends | — | **on** |
| **Invalid packets** | Packets conntrack cannot match to a connection | — | **on** |
| **Fragment drop** | Fragmented **IPv4** packets | — | off |
| **Bogon filter** | Impossible **IPv4** source addresses on a non-loopback interface | — | off |
| **Connection limit** | Simultaneous connections from one source above its cap | `connection_limit_max` — 100 | off |
| **TCP RST flood** | Inbound RST packets from one source above its rate | `tcp_rst_flood_limit` — 100/s | off |

> **Every rate is counted per source address.** The kernel keeps one counter per
> address, in a set whose entries expire when that source goes quiet, so a flood from
> one host cannot consume the budget that keeps another host — or you — connected.
>
> **This was not true before 2.5.0.** Four of these modules held a single counter for
> all traffic, while the interface, this page and the JSON schema all described a
> per-source rate. Five SSH connection attempts a minute from anywhere exhausted the
> budget, and every further SSH connection was dropped, the administrator's included:
> the module meant to prevent a lockout produced one, from a single attacker, at
> negligible cost.

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

> **Not for hosts behind NAT.** On a cloud instance or a LAN, RFC 1918 *is* the real
> network and this filter drops your own traffic. Same for container hosts — see
> [Docker coexistence]({{ '/features/docker/' | relative_url }}).
>
> **Not for a DHCP server either.** A client requesting a lease has no address yet and
> sends from `0.0.0.0`, which this filter drops.

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

> **None of this worked before 2.5.0.** Eight of these switches produced no rule
> at all, and the one that did carried no prefix: the log expression's `Key`
> field is a bitmask over attribute indices and was being set to a bare
> attribute number, so the kernel received an empty log group instead. The
> command above matched nothing, whatever was switched on.

This is the *kernel* log — packets. Administrative changes are in the
[audit log]({{ '/features/audit-log/' | relative_url }}) instead.

## Which to turn on

| Host | Turn on | Leave off |
|---|---|---|
| Public server, static address | Everything under Attack protection, plus the bogon filter | Fragment drop, unless you know your traffic |
| Behind NAT, or on a LAN | SSH brute-force, SYN flood, port scan, invalid packets | Bogon filter, broadcast/multicast/anycast |
| Container host | The defaults | Bogon filter — bridge ranges are RFC 1918 |

The interface writes the same `[firewall]` section you would edit by hand; every key
is listed under [Configuration]({{ '/configuration/' | relative_url }}).
