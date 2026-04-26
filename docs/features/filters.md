---
layout: default
title: Firewall Filters
description: Optional protection modules in easywall — SYN flood, port scan, bogon filter, and more.
---

# Firewall Filters

easywall includes a set of optional protection modules, configured in `/etc/easywall/easywall.toml` under `[firewall]`.

All filters are implemented directly in nftables via the `inet easywall` table — no subprocess, no shell, no injection risk.

## Always Active

These rules are always in effect and cannot be disabled:

| Rule | Description |
|---|---|
| Default DROP | All incoming traffic not explicitly allowed is dropped |
| Loopback ACCEPT | Traffic on the loopback interface (`lo`) is always allowed |
| RELATED/ESTABLISHED | Responses to outbound connections are allowed |
| ICMPv4 (types 0,3,11,12) | Echo reply, unreachable, time exceeded, parameter problem |
| ICMPv6 (types 1,2,3,4,128,129) | Required for IPv6 to function correctly |

## Optional Modules

### SSH Brute-Force Protection (`ssh_brute_force`)

Rate-limits new connections to SSH ports. Ports must be tagged as **SSH** in the Ports UI.

### ICMP Flood Protection (`icmp_flood`)

Rate-limits ICMP echo requests per source IP to prevent ping floods.

### SYN Flood Protection (`syn_flood`)

Rate-limits new TCP SYN packets per source IP.

### Port Scan Detection (`port_scan`)

Drops TCP packets with suspicious flag combinations: NULL, FIN, XMAS, SYN+FIN, and others used in stealth scans.

### Invalid Packet Drop (`drop_invalid_packets`)

Drops packets in the `INVALID` conntrack state, which cannot belong to any known connection.

### Fragment Drop (`drop_fragments`)

Drops IP-fragmented packets, which are commonly used in evasion attacks.

### Bogon Filter (`bogon_filter`)

Drops packets from private/loopback/link-local address ranges (RFC 1918) arriving on external interfaces. Prevents IP spoofing attacks.

### Connection Limit (`connection_limit_per_ip`)

Limits the number of simultaneous connections from a single source IP.

### TCP RST Flood Protection (`tcp_rst_flood`)

Rate-limits TCP RST packets, which can be used to disrupt established connections.

### Broadcast / Multicast Drop

Drops packets with broadcast or multicast destination addresses on the input chain.
