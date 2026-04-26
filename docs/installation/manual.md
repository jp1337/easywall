---
layout: default
title: Manual Installation
description: Build easywall from source and install manually on any Linux distribution.
---

# Manual Installation

Use this method if you want full control over the installation, are on a non-Debian Linux distribution, or need to build from source.

## Requirements

See [Requirements]({{ '/installation/requirements/' | relative_url }}) first.

## Build from Source

### Prerequisites

Go 1.25 or newer is required on the build machine (not on the target server):

```bash
# Check your Go version
go version

# Install Go if needed — https://go.dev/dl/
# or via your package manager:
sudo apt-get install golang-go   # Debian/Ubuntu (may be older version)
```

### Clone and Build

```bash
git clone https://github.com/jp1337/easywall.git
cd easywall

# Build with version info embedded
VERSION=$(git describe --tags --always --dirty)
go build \
  -ldflags "-s -w -X github.com/jp1337/easywall/internal/shared.CurrentVersion=$VERSION" \
  -o easywall-core ./cmd/easywall-core
go build \
  -ldflags "-s -w -X github.com/jp1337/easywall/internal/shared.CurrentVersion=$VERSION" \
  -o easywall-web ./cmd/easywall-web
```

### Cross-Compile for a Different Architecture

```bash
# For a Raspberry Pi (arm64)
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build \
  -o easywall-core-arm64 ./cmd/easywall-core
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build \
  -o easywall-web-arm64 ./cmd/easywall-web
```

Transfer the binaries to the target machine via scp/rsync.

## Install Binaries and Assets

```bash
# Copy binaries
sudo install -m 0755 easywall-core /usr/sbin/easywall-core
sudo install -m 0755 easywall-web  /usr/sbin/easywall-web

# Create the service user and group
sudo groupadd --system easywall 2>/dev/null || true
sudo useradd  --system --no-create-home --shell /usr/sbin/nologin \
              --gid easywall easywall 2>/dev/null || true

# Create directory structure
sudo install -d -m 0750 -o root     -g easywall /run/easywall
sudo install -d -m 0750 -o root     -g root     /etc/easywall
sudo install -d -m 0750 -o root     -g root     /etc/easywall/ssl
sudo install -d -m 0750 -o easywall -g easywall /var/lib/easywall
sudo install -d -m 0750 -o root     -g easywall /var/log/easywall

# Copy assets (web templates, locales) and configs
sudo cp -r web     /usr/share/easywall/web
sudo cp -r locales /usr/share/easywall/locales
sudo cp config/easywall.toml /etc/easywall/easywall.toml
```

## Configure Secrets

The web configuration requires two 32-byte random hex secrets for session signing and CSRF protection. Generate them:

```bash
SESSION_KEY=$(openssl rand -hex 32)
CSRF_KEY=$(openssl rand -hex 32)
```

Write `web.toml` with the generated values:

```bash
sudo tee /etc/easywall/web.toml > /dev/null <<EOF
bind_addr   = "0.0.0.0:12227"
socket_path = "/run/easywall/core.sock"
ssl_dir     = "/etc/easywall/ssl"
data_dir    = "/var/lib/easywall"
language    = "en"

session_key = "${SESSION_KEY}"
csrf_key    = "${CSRF_KEY}"

username = ""
password = ""

[tls]
cert = ""
key  = ""
EOF
```

```bash
sudo chown root:easywall /etc/easywall/web.toml
sudo chmod 0640 /etc/easywall/web.toml
sudo chown root:easywall /etc/easywall/easywall.toml
sudo chmod 0640 /etc/easywall/easywall.toml
```

## systemd Services

```bash
sudo cp systemd/easywall-core.service /etc/systemd/system/
sudo cp systemd/easywall-web.service  /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now easywall-core easywall-web
```

Check that both services started:

```bash
systemctl status easywall-core easywall-web
```

Both should show `active (running)`.

## First Run

Open `https://<your-server>:12227` in your browser. Your browser will warn about the self-signed certificate — this is expected on a fresh install. Accept the security exception and complete the **First-Run Wizard** to set your username and password.

## Uninstall

```bash
# Stop and disable services
sudo systemctl stop    easywall-core easywall-web
sudo systemctl disable easywall-core easywall-web
sudo rm -f /etc/systemd/system/easywall-core.service \
           /etc/systemd/system/easywall-web.service
sudo systemctl daemon-reload

# Remove binaries and assets
sudo rm -f /usr/sbin/easywall-core /usr/sbin/easywall-web
sudo rm -rf /usr/share/easywall

# Remove config and data (optional — backup first if needed)
sudo rm -rf /etc/easywall /var/lib/easywall /var/log/easywall

# Remove the easywall nftables table
sudo nft delete table inet easywall 2>/dev/null || true

# Remove user/group
sudo userdel easywall 2>/dev/null || true
sudo groupdel easywall 2>/dev/null || true
```

## Troubleshooting

**`easywall-core` fails to start: `permission denied`**

The core daemon needs `CAP_NET_ADMIN` to write nftables rules. Check the systemd service runs as root:
```bash
systemctl cat easywall-core | grep User
```

**`easywall-web` cannot connect to core socket**

The Unix socket `/run/easywall/core.sock` must be accessible to the `easywall` group. Check:
```bash
ls -la /run/easywall/core.sock
# Should show: srw-rw---- root easywall
```

**`nft: No such file or directory`**

Install nftables: `sudo apt-get install nftables`

**TLS certificate errors in browser**

On first start, a self-signed certificate is generated in `/etc/easywall/ssl/`. To use your own certificate, set paths in `/etc/easywall/web.toml`:
```toml
[tls]
cert = "/etc/letsencrypt/live/example.com/fullchain.pem"
key  = "/etc/letsencrypt/live/example.com/privkey.pem"
```
Restart `easywall-web` after changes.
