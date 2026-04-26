---
layout: default
title: Blacklist & Whitelist
description: Block and allow IP addresses and CIDRs in easywall.
---

# Blacklist & Whitelist

## Blacklist

IPs and CIDRs on the blacklist are **always blocked**, regardless of any open port rules.

Traffic from blacklisted addresses is dropped before reaching any other rule, and optionally logged with the prefix `easywall blacklist:`.

**Supported formats:**

```
192.168.1.100
10.0.0.0/8
2001:db8::/32
```

## Whitelist

IPs and CIDRs on the whitelist are **always allowed**, bypassing all port, blacklist, and protection-module rules.

Use this for trusted management IPs (e.g. your own static IP) to ensure you are never locked out.

<div class="callout callout-warning">
  <svg class="callout-icon" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd"/></svg>
  <div class="callout-content">
    <strong>Warning</strong>
    <p>Whitelisted IPs bypass <strong>all</strong> firewall rules including the blacklist. Only add addresses you fully trust.</p>
  </div>
</div>

## Apply

Changes to the blacklist and whitelist are staged. Activate them via the **Apply** page with the two-step confirmation.
