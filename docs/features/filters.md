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

{% include themed-figure.html base="/assets/diagrams/rule-order" ext="svg"
   alt="Decision flow for an incoming packet: loopback, established connections and ICMP first, then protection modules, then Docker bridge networks, then the blacklist which drops, then the whitelist which accepts every port, then open ports, then custom rules, and finally the chain policy which drops." %}

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

## Attack protection

| Module | Drops | Tuning | Default |
|---|---|---|---|
| **SSH brute-force** | New SSH connections above a per-source rate. Applies to ports marked *SSH protection* on the [ports page]({{ '/features/ports/' | relative_url }}), and to 22 if none is marked | `ssh_brute_force_connection_limit` — 5 | **on** |
| **ICMP flood** | Echo requests above a per-source rate | `icmp_flood_connection_limit` — 10 | **on** |
| **SYN flood** | New TCP connections above a rate | `syn_flood_limit` — 100/s | **on** |
| **Port scan detection** | NULL, FIN and XMAS flag combinations — packets no real client sends | — | **on** |
| **Invalid packets** | Packets conntrack cannot match to a connection | — | **on** |
| **Fragment drop** | IP-fragmented packets | — | off |
| **Bogon filter** | Private and special-use source addresses on a non-loopback interface | — | off |
| **Connection limit** | Concurrent connections above a per-source cap | `connection_limit_max` — 100 | off |
| **TCP RST flood** | Inbound RST packets above a rate | `tcp_rst_flood_limit` — 100/s | off |

### What the bogon filter drops

A packet claiming to come from one of these, arriving on a real interface, is
spoofed — nothing on the public internet legitimately has such a source address.

| Range | | Range | |
|---|---|---|---|
| `0.0.0.0/8` | "this network" | `169.254.0.0/16` | link-local |
| `10.0.0.0/8` | private | `172.16.0.0/12` | private |
| `100.64.0.0/10` | carrier NAT | `192.168.0.0/16` | private |
| `127.0.0.0/8` | loopback | `192.0.2.0/24`, `198.51.100.0/24` | documentation |

> **Not for hosts behind NAT.** On a cloud instance or a LAN, RFC 1918 *is* the real
> network and this filter drops your own traffic. Same for container hosts — see
> [Docker coexistence]({{ '/features/docker/' | relative_url }}).

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
| `<module>_log` | Drops by that one module | `easywall` |
| `log_blocked_connections` | Everything the final policy drops | `easywall drop:` |
| `log_blacklist_connections` | Blacklist hits, before the drop | `easywall blacklist:` |

```bash
journalctl -k -f | grep easywall
```

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
