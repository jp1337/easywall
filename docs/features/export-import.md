---
layout: default
title: Export & Import
description: Export and import easywall firewall rules as JSON — backups, server migration, version control, and automation.
---

# Export & Import

easywall can export and import all firewall rules as a single JSON file. Use this for backups before major changes, migrating rule sets between servers, and keeping your firewall configuration in version control.

## Exporting Rules

1. Navigate to **Dashboard**
2. Click **Export Rules**
3. Your browser downloads a file named `easywall-rules-YYYY-MM-DD.json`

The exported file contains the **staged** rule set — the current state of all pending changes. If you want to export the actively running rules, apply your staged changes first, then export.

## Importing Rules

1. Navigate to **Dashboard**
2. Click **Import Rules**
3. Select a previously exported `.json` file
4. easywall validates the file contents — invalid port numbers, malformed IPs, and unknown protocols are rejected
5. On success, the rules are loaded as **staged** rules
6. Go to **Apply** to activate them with the two-step confirmation

<div class="callout callout-warning">
  <svg class="callout-icon" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd"/></svg>
  <div class="callout-content">
    <strong>Warning</strong>
    <p>Importing <strong>replaces all staged rules</strong>. The currently running (applied) firewall rules are not changed until you explicitly apply the import.</p>
  </div>
</div>

## File Format

The export format is a flat JSON object with one array per rule type:

```json
{
  "tcp": [
    {"port": "22",  "description": "SSH",   "ssh": true},
    {"port": "80",  "description": "HTTP",  "ssh": false},
    {"port": "443", "description": "HTTPS", "ssh": false}
  ],
  "udp": [
    {"port": "53", "description": "DNS", "ssh": false}
  ],
  "blacklist": [
    "192.0.2.1",
    "198.51.100.0/24"
  ],
  "whitelist": [
    "203.0.113.10"
  ],
  "forwarding": [
    {"protocol": "tcp", "source_port": 2222, "dest_port": 22}
  ],
  "custom": [
    "iif eth0 ip protocol udp udp dport 1194 accept"
  ]
}
```

### Field Reference

| Field | Type | Description |
|---|---|---|
| `tcp` | array | TCP port rules — `port` (string), `description` (string), `ssh` (bool) |
| `udp` | array | UDP port rules — same structure as tcp |
| `blacklist` | array of strings | Blocked IPs/CIDRs (IPv4 and IPv6) |
| `whitelist` | array of strings | Trusted IPs/CIDRs (bypass all rules) |
| `forwarding` | array | Port forwarding rules — `protocol`, `source_port`, `dest_port` |
| `custom` | array of strings | Raw nftables match expressions appended to the input chain |

All fields are optional — an empty array `[]` means no rules for that type.

## Use Cases

### Server Migration

1. Export rules from the old server
2. Transfer the JSON file to the new server
3. Import on the new server
4. Apply with two-step confirmation — verify connectivity before confirming

### Pre-Change Backup

Before making significant changes, export the current rules. If something goes wrong, you can re-import from the backup without manually recreating every rule.

### Version Control

Commit your exported JSON to git alongside your Ansible playbooks or Terraform configs:

```bash
# Export via API or manually, then commit
git add easywall-rules-production.json
git commit -m "chore: update firewall rules — open port 8080 for staging"
```

### Automation via Command Line

The rules JSON can be constructed programmatically and POSTed to the web API. Combine with your infrastructure-as-code workflow:

```bash
# Example: generate rules.json from a template and import via curl
curl -k -b cookies.txt \
  -F "file=@rules.json" \
  https://server:12227/import
```

## Validation

easywall validates every import before staging it. Rejected conditions:

- Port numbers outside 1–65535
- Malformed CIDR notation (e.g. `192.168.1.1/33`)
- Invalid IP addresses
- Unknown forwarding protocol (only `tcp` and `udp` are accepted)
- Forwarding ports outside valid range

Validation errors are shown inline — no partial imports occur.
