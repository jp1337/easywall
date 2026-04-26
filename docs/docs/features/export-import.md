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

!!! warning
    Importing replaces all staged rules. Current (applied) rules are not affected until you apply.

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
