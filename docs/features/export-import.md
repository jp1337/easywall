---
layout: default
title: Export & Import
description: The whole rule set as one JSON file — for migration, for a backup before a risky change, or for version control.
---

# Export & Import

The buttons sit top-right on the dashboard. Export downloads the **staged** rule set;
import replaces it. Neither touches the running firewall — that still needs an
[apply]({{ '/features/apply/' | relative_url }}).

| | |
|---|---|
| **Export** | Downloads `easywall-rules-<date>.json` |
| **Import** | Validates, then replaces the staged set entirely — it is not a merge |

> **Import is a replacement.** Anything staged that is not in the file is gone after
> importing. Export first if you are unsure.

## The format

One array per rule type. Every field is optional; an absent or empty array means no
rules of that kind.

```json
{
  "tcp": [
    {"port": "22",  "description": "SSH",   "ssh": true},
    {"port": "443", "description": "HTTPS", "ssh": false}
  ],
  "udp":        [{"port": "53", "description": "DNS", "ssh": false}],
  "blacklist":  ["192.0.2.1", "198.51.100.0/24"],
  "whitelist":  ["203.0.113.10"],
  "forwarding": [{"protocol": "tcp", "source_port": 2222, "dest_port": 22}],
  "custom":     ["iif eth0 ip protocol udp udp dport 1194 accept"]
}
```

| Key | Shape |
|---|---|
| `tcp`, `udp` | `port` string, `description` string, `ssh` bool |
| `blacklist`, `whitelist` | strings — IPv4, IPv6 or CIDR |
| `forwarding` | `protocol` `"tcp"` or `"udp"`, `source_port` int, `dest_port` int |
| `custom` | strings — raw nftables match expressions |

## What gets rejected

Import validates before staging anything. **A file with one bad entry imports
nothing.**

| Refused | Example | Why |
|---|---|---|
| A port outside 1–65535 | `70000` | |
| A malformed CIDR | `192.168.1.1/33` | |
| An address that does not parse | `192.0.2.999` | |
| A forwarding protocol that is not `tcp` or `udp` | `sctp` | |
| A custom rule containing a newline or a semicolon | | nft reads both as the end of one command, so it would be a **second command run by the root daemon**. The editor splits on newlines and never produced one; a file can |
| A custom rule `nft --check` rejects | | the editor has always checked as you type; imported rules were not checked at all |
| A file over **512 KB** | | room for several thousand addresses. A larger set is easier to place in `rules.json` directly than to push through a browser upload |

## Worth doing

| | |
|---|---|
| **Before a risky change** | Export. Re-importing beats recreating twenty rules by hand |
| **Migrating a server** | Export, transfer, import, apply — and verify before confirming |
| **Version control** | The JSON is stable and diffs cleanly next to your Ansible or Terraform |

```bash
git add easywall-rules-production.json
git commit -m "chore(firewall): open 8080 for staging"
```
