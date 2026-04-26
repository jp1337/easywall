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

!!! warning
    Whitelisted IPs bypass **all** firewall rules including the blacklist. Only add addresses you fully trust.

## Apply

Changes to the blacklist and whitelist are staged. Activate them via the **Apply** page with the two-step confirmation.
