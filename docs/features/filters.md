---
layout: default
title: Firewall Filters
description: Optional protection modules in easywall — SYN flood, port scan, bogon filter, connection limits, and more.
---

# Firewall Filters

easywall ships with a set of optional protection modules that harden the host beyond simply opening and closing ports. They are configured in `/etc/easywall/easywall.toml` under the `[firewall]` section.

All filters are implemented as native nftables rules in the `inet easywall` table — no subprocess, no shell commands, no injection risk. Changes take effect only after **Apply** with the two-step confirmation.

---

## Always-Active Rules

These rules are compiled into every ruleset and cannot be disabled:

| Rule | nftables expression | Why it exists |
|---|---|---|
| Default DROP | `policy drop` on `input` chain | Deny-by-default posture |
| Loopback ACCEPT | `iif lo accept` | Local process communication always works |
| RELATED/ESTABLISHED | `ct state {related, established} accept` | Return traffic for outbound connections |
| ICMPv4 (0,3,11,12) | `ip protocol icmp icmp type {...} accept` | Echo reply, unreachable, TTL exceeded, param problem |
| ICMPv6 (1,2,3,4,128,129) | `icmpv6 type {...} accept` | Minimum set required for IPv6 to function |
| ICMPv6 ND (if enabled) | types 133–136 | Router/neighbor discovery for address autoconfiguration |

---

## Optional Modules

### SSH Brute-Force Protection (`ssh_brute_force`)

Creates a named `sshbrute` chain. TCP ports marked as **SSH** in the Ports UI are redirected through this chain.

**What it does:** limits new connection attempts per source IP. Hosts that exceed the threshold have new connections dropped until the rate drops.

```toml
ssh_brute_force                  = true
ssh_brute_force_connection_limit = 5     # max new connections in the rate window
ssh_brute_force_log              = false # log dropped attempts to kernel log
ssh_brute_force_log_limit        = 60    # rate-limit log entries (per minute)
```

**When to use:** always, for any SSH port. The protection is negligible overhead for legitimate users but makes credential stuffing impractical.

---

### ICMP Flood Protection (`icmp_flood`)

Rate-limits ICMP echo requests (`ping`) per source IP to prevent bandwidth exhaustion.

```toml
icmp_flood                       = true
icmp_flood_connection_limit      = 10    # max pings per second per source IP
icmp_flood_log                   = false
icmp_flood_log_limit             = 60
```

**When to use:** almost always. Disabling this allows a single host to flood your server with pings at full network speed.

---

### SYN Flood Protection (`syn_flood`)

Rate-limits new TCP SYN packets per source IP. A SYN flood exhausts the server's TCP connection state table, causing denial of service for legitimate clients.

```toml
syn_flood                        = true
syn_flood_log                    = false
syn_flood_limit                  = 100   # max new SYN packets per second per source IP
```

**When to use:** any server exposed to the internet. SYN flooding is one of the most common DDoS techniques.

---

### Port Scan Detection (`port_scan`)

Creates a named `portscan` chain. Drops TCP packets with suspicious flag combinations used by network scanners (Nmap, etc.):

| Scan Type | Flags Set | Flags Clear |
|---|---|---|
| NULL scan | none | — |
| FIN scan | FIN | SYN, RST, ACK |
| XMAS scan | FIN, PSH, URG | SYN, RST, ACK |
| SYN+FIN | SYN + FIN | — |
| SYN+RST | SYN + RST | — |
| FIN+RST | FIN + RST | — |
| PSH alone | PSH | SYN, ACK |
| URG alone | URG | SYN, ACK |

```toml
port_scan                        = true
port_scan_log                    = false
```

**When to use:** internet-facing servers. Note: some of these patterns are also used by broken TCP implementations — if you see legitimate traffic dropped, check logs with `port_scan_log = true`.

---

### Invalid Packet Drop (`drop_invalid_packets`)

Drops packets in the `INVALID` conntrack state. These packets have no known connection entry and do not match any expected connection state — they are often used in TCP reset attacks or are remnants of expired connections.

```toml
drop_invalid_packets             = true
drop_invalid_packets_log         = false
```

**When to use:** almost always. There are very few legitimate reasons for a packet to be in `INVALID` state on a server.

---

### Fragment Drop (`drop_fragments`)

Drops IP-fragmented packets. IP fragmentation is rarely needed in modern networks (MTU discovery handles path sizing) and is commonly used in evasion attacks that split exploit payloads across fragments.

```toml
drop_fragments                   = false
drop_fragments_log               = false
```

**When to use:** production servers on well-configured networks. Avoid on networks with legacy devices or MTU issues — fragmentation may be legitimate there.

---

### Bogon Filter (`bogon_filter`)

Drops inbound packets whose **source IP** belongs to private, loopback, link-local, or reserved address ranges (RFC 1918 / RFC 5735). Such packets on a public interface indicate IP spoofing.

Filtered ranges:

| CIDR | Type |
|---|---|
| `0.0.0.0/8` | "This network" |
| `10.0.0.0/8` | RFC 1918 private |
| `100.64.0.0/10` | Shared address space (RFC 6598) |
| `127.0.0.0/8` | Loopback |
| `169.254.0.0/16` | Link-local |
| `172.16.0.0/12` | RFC 1918 private |
| `192.0.2.0/24` | Documentation (TEST-NET-1) |
| `192.168.0.0/16` | RFC 1918 private |
| `198.51.100.0/24` | Documentation (TEST-NET-2) |

```toml
bogon_filter                     = false
bogon_filter_log                 = false
```

**When to use:** servers with a single public IP, no private network interfaces that carry legitimate traffic, and no VPNs that use RFC 1918 addressing. **Do not enable** if your server is behind a NAT (e.g., cloud instances where RFC 1918 is the actual LAN) or has a private management interface.

---

### Connection Limit per Source IP (`connection_limit_per_ip`)

Limits the total number of simultaneous active connections from a single source IP. This prevents a single host from monopolising all connection state.

```toml
connection_limit_per_ip          = false
connection_limit_max             = 100    # max simultaneous connections per source IP
```

**When to use:** web servers, API servers, and any public-facing service where you want to prevent a single client from consuming all connection state. Tune `connection_limit_max` to a value above your expected legitimate maximum.

---

### TCP RST Flood (`tcp_rst_flood`)

Rate-limits inbound TCP RST packets per source IP. Sending forged RST packets is used to terminate established connections (TCP reset attacks).

```toml
tcp_rst_flood                    = false
tcp_rst_flood_log                = false
tcp_rst_flood_limit              = 100    # max RST packets per second per source IP
```

**When to use:** any server where connection teardown attacks are a concern (e.g., long-lived connection services like SSH, VPN endpoints, database proxies).

---

### Broadcast Drop (`drop_broadcast`)

Drops packets with a broadcast destination address on the input chain. Broadcast traffic from external networks is almost always unwanted on a server.

```toml
drop_broadcast                   = false
```

---

### Multicast Drop (`drop_multicast`)

Drops packets with a multicast destination address on the input chain.

```toml
drop_multicast                   = false
```

**When to use:** enable if you do not use any multicast protocols (mDNS, SSDP, etc.).

---

## Logging

Two global logging options add audit-trail rules:

### Block Logging (`log_blocked_connections`)

Adds a rate-limited `log` rule just before the final `drop` in the input chain. All dropped traffic is logged to the kernel journal with the prefix `easywall drop:`.

```toml
log_blocked_connections       = false
log_blocked_connections_limit = 60     # log entries per minute (burst)
```

View logs: `journalctl -k | grep "easywall drop:"`

### Blacklist Logging (`log_blacklist_connections`)

Logs packets matched by the blacklist before dropping them.

```toml
log_blacklist_connections       = false
log_blacklist_connections_limit = 60
```

View logs: `journalctl -k | grep "easywall blacklist:"`

---

## Recommended Configuration

For a typical internet-facing Linux server:

```toml
[firewall]
ssh_brute_force       = true
icmp_flood            = true
syn_flood             = true
port_scan             = true
drop_invalid_packets  = true
drop_fragments        = false
bogon_filter          = false   # enable only if no private NICs / no cloud NAT
connection_limit_per_ip = false
tcp_rst_flood         = false
drop_broadcast        = false
drop_multicast        = false
log_blocked_connections = false  # enable temporarily for debugging
```
