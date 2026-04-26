---
layout: default
title: Export & Import
description: Export and import easywall firewall rules as JSON — backups, migration, version control.
---

# Export & Import

easywall can export and import all firewall rules as a single JSON file. Use this for backups, migrating between servers, or version-controlling your rule sets.

## Export

1. Navigate to **Dashboard**
2. Click **Export Rules**
3. A JSON file named `easywall-rules-YYYY-MM-DD.json` is downloaded

The exported file contains all rule sets (TCP/UDP ports, blacklist, whitelist, forwarding, custom rules) in the staged state.

## Import

1. Navigate to **Dashboard**
2. Click **Import Rules**
3. Select a previously exported `.json` file
4. The rules are validated and loaded as staged rules
5. Go to **Apply** to activate them with the two-step confirmation

<div class="callout callout-warning">
  <svg class="callout-icon" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd"/></svg>
  <div class="callout-content">
    <strong>Warning</strong>
    <p>Importing replaces all staged rules. Current (applied) rules are not affected until you apply.</p>
  </div>
</div>

## File Format

```json
{
  "tcp": [
    {"port": "22", "description": "SSH", "ssh": true},
    {"port": "443", "description": "HTTPS", "ssh": false}
  ],
  "udp": [],
  "blacklist": ["192.168.1.100"],
  "whitelist": [],
  "forwarding": [],
  "custom": []
}
```
