# Port Management

The **Ports** page lets you open TCP and UDP ports in the firewall.

## Adding a Port Rule

1. Navigate to **Ports** in the sidebar
2. Select the protocol tab (**TCP** or **UDP**)
3. Enter the port number or range and an optional description
4. Click **Save**
5. Go to **Apply** to activate the new rules

### Port Formats

| Format | Example | Description |
|---|---|---|
| Single port | `80` | Opens exactly one port |
| Port range | `8000:9000` | Opens all ports from 8000 to 9000 (inclusive) |

## SSH Protection

Ports marked as **SSH** are routed through the SSH brute-force protection chain, which rate-limits new connection attempts per source IP. Enable this for your SSH port (default: 22).

## Staged Changes

Changes on the Ports page are saved as **staged** rules. They do not affect the running firewall until you go to the **Apply** page and confirm activation.
