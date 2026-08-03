---
layout: default
title: Blacklist & Whitelist
description: Block and allow IP addresses and CIDR ranges in easywall — rule ordering, use cases, and best practices.
---

# Blacklist & Whitelist

## Blacklist

IPs and CIDRs on the blacklist are **always blocked**, regardless of any open port rules or protection modules.

Traffic from blacklisted addresses is dropped immediately before reaching any port rule, whitelist entry, or protection chain. Optionally, blocked traffic can be logged with the prefix `easywall blacklist:` (see [Configuration]({{ '/configuration/' | relative_url }})).

**Supported formats:**

```
192.168.1.100          # single IPv4 address
10.0.0.0/8             # IPv4 CIDR range
2001:db8::/32          # IPv6 CIDR range
```

The editor performs **live syntax validation** as you type — invalid lines are reported by line number under the textarea before you save. Comments (lines starting with `#`) and blank lines are skipped silently.

### When to Use the Blacklist

- Block a specific IP address that is scanning or attacking your server
- Block entire country-level or ISP-level ranges you never expect traffic from
- Block Tor exit nodes or known botnet CIDRs
- Block IPs that repeatedly fail authentication

<div class="callout callout-info">
  <svg class="callout-icon" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a.75.75 0 000 1.5h.253a.25.25 0 01.244.304l-.459 2.066A1.75 1.75 0 0010.747 15H11a.75.75 0 000-1.5h-.253a.25.25 0 01-.244-.304l.459-2.066A1.75 1.75 0 009.253 9H9z" clip-rule="evenodd"/></svg>
  <div class="callout-content">
    <strong>Tip</strong>
    <p>For broad blocking (e.g., entire countries), use the largest CIDR that covers the range. A single <code>/16</code> entry is more efficient than 256 individual <code>/24</code> entries.</p>
  </div>
</div>

---

## Whitelist

IPs and CIDRs on the whitelist are accepted before the port rules are consulted, so a whitelisted address reaches any port — including one you never opened — and skips the protection modules.

It does **not** override the blacklist. As the [rule ordering](#rule-ordering) below shows, the blacklist is evaluated first, so an address present in both lists is dropped.

Use this for trusted management addresses to ensure you are never locked out, even if you accidentally close the SSH port or trigger a rate-limit rule.

**Supported formats** (same as blacklist):

```
203.0.113.42           # your static public IP
10.0.0.0/24            # your internal management network
2001:db8::1            # IPv6 management address
```

The whitelist editor uses the same live IP/CIDR validation as the blacklist — you'll see a list of invalid lines under the textarea before saving.

### When to Use the Whitelist

- Your own static public IP address for administration
- Office or VPN subnet for team access
- Monitoring system IP (Nagios, Zabbix, Datadog agent)
- Internal network ranges that need unrestricted access

<div class="callout callout-warning">
  <svg class="callout-icon" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd"/></svg>
  <div class="callout-content">
    <strong>Warning</strong>
    <p>Whitelisted IPs reach every port and skip the protection modules, including SSH brute-force protection and connection limits. They do not bypass the blacklist, which is evaluated first. Only add addresses you fully trust and control.</p>
  </div>
</div>

---

## Rule Ordering

easywall evaluates rules in a fixed order. Understanding this order helps predict behaviour when multiple rules could match a packet:

```
1. Loopback (lo) — always ACCEPT
2. RELATED / ESTABLISHED — always ACCEPT
3. ICMP (v4 + v6 base types) — always ACCEPT
4. Optional protection chains (SYN flood, port scan, …)
5. Docker bridge networks (if Docker mode enabled)
6. Blacklist — DROP matching source IPs
7. Whitelist — ACCEPT matching source IPs
8. Open ports (TCP / UDP rules)
9. Final log rule (if log_blocked_connections = true)
10. Default DROP (everything else)
```

A packet blocked at step 6 (blacklist) never reaches the open port rules at step 8 — even if the destination port is in the port list.

A packet matching the whitelist at step 7 is accepted immediately and skips the port rules entirely. This means a whitelisted IP can reach *any* port, including closed ones.

---

## Applying Changes

All blacklist and whitelist changes are staged. They do not affect the running firewall until you visit **Apply** and confirm the changes within the acceptance window. If you do not confirm, the old rules are automatically restored.

---

## Troubleshooting

**My IP is still being blocked after removing it from the blacklist**

Staged changes are not active yet — go to **Apply** and confirm.

**I accidentally whitelisted a range that is too broad**

Remove the entry, save, then Apply with the two-step confirmation. If you locked yourself out before applying, the auto-rollback will restore the previous rules after the acceptance timeout.

**I cannot find the right CIDR for a range**

Use an online CIDR calculator, or check the BGP prefix for the IP with a tool like:
```bash
whois 198.51.100.42 | grep route
```
