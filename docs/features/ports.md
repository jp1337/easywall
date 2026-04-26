---
layout: default
title: Port Management
description: Open TCP and UDP ports in easywall — single ports, ranges, SSH brute-force protection, and best practices.
---

# Port Management

The **Ports** page manages which TCP and UDP ports are open on the firewall. All other incoming traffic is blocked by default.

## Adding a Port Rule

1. Navigate to **Ports** in the sidebar
2. Select the protocol tab — **TCP** or **UDP**
3. Enter the port number or range in the **Port** field
4. Optionally enter a human-readable **Description** (e.g. `HTTPS`, `Game server`)
5. Check **SSH** if this port runs an SSH server (see [SSH Protection](#ssh-protection))
6. Click **Save**
7. Go to **Apply** to activate the new rules with two-step confirmation

### Port Formats

| Format | Example | Description |
|---|---|---|
| Single port | `80` | Opens exactly one port |
| Port range | `8000:9000` | Opens all ports from 8000 to 9000 (inclusive) |

Port numbers must be between `1` and `65535`. Ranges must be in ascending order (`low:high`).

## SSH Protection

Any port marked as **SSH** is routed through the SSH brute-force protection chain. This chain rate-limits new incoming connection attempts per source IP to prevent dictionary and credential-stuffing attacks.

The rate limits are configured in `/etc/easywall/easywall.toml`:

```toml
[firewall]
ssh_brute_force                  = true
ssh_brute_force_connection_limit = 5     # max new connections per time window
ssh_brute_force_log              = false # log blocked attempts
```

**Always mark your SSH port as SSH**, even if it is not port 22. The protection chain is applied per-port, so it works for non-standard SSH ports too.

## Staged Changes

Changes on the Ports page are saved as **staged** rules. They do not affect the running firewall until you go to the **Apply** page and confirm activation. This allows you to prepare multiple rule changes in one batch and apply them atomically.

## Common Setups

### Web Server

| Protocol | Port | Description | SSH |
|---|---|---|---|
| TCP | `80` | HTTP (redirect to HTTPS) | ☐ |
| TCP | `443` | HTTPS | ☐ |

### Mail Server

| Protocol | Port | Description | SSH |
|---|---|---|---|
| TCP | `25` | SMTP | ☐ |
| TCP | `587` | SMTP Submission | ☐ |
| TCP | `465` | SMTPS | ☐ |
| TCP | `993` | IMAPS | ☐ |
| TCP | `995` | POP3S | ☐ |

### Game Server (example: Minecraft)

| Protocol | Port | Description | SSH |
|---|---|---|---|
| TCP | `25565` | Minecraft Java | ☐ |
| UDP | `25565` | Minecraft Bedrock | ☐ |

### SSH on Non-Standard Port

| Protocol | Port | Description | SSH |
|---|---|---|---|
| TCP | `2222` | SSH | ☑ |

## Deleting a Port Rule

Click the **✕** button next to any rule in the list to stage its deletion. The rule stays active until you Apply and confirm.

## Troubleshooting

**Port is open but connections are refused**

The firewall allows the packet, but no process is listening on that port. Check with:
```bash
ss -tlnp | grep <port>
```

**Port is in the list but still blocked**

Staged changes have not been applied yet. Go to **Apply** and confirm.

**SSH connection drops after Apply**

The two-step activation is designed to prevent this. After Apply you have a confirmation window (default: 120 seconds) to verify connectivity from a second terminal. If you do nothing, the old rules are automatically restored.

**Rate-limited by SSH protection**

Your IP may have hit the connection rate limit. Wait a few minutes, then reconnect. To whitelist your management IP permanently, add it to the [Whitelist]({{ '/features/blacklist/' | relative_url }}).
