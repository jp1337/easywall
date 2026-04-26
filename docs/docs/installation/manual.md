# Manual Installation

Use this method if you want full control over the installation or are on a non-Debian Linux distribution.

## Requirements

See [Requirements](requirements.md) first.

## Build from Source

```bash
git clone https://github.com/jpylypiw/easywall.git
cd easywall

# Build both binaries
go build -o easywall-core ./cmd/easywall-core
go build -o easywall-web  ./cmd/easywall-web
```

Cross-compile for a different architecture:

```bash
GOOS=linux GOARCH=arm64 go build -o easywall-core ./cmd/easywall-core
GOOS=linux GOARCH=arm64 go build -o easywall-web  ./cmd/easywall-web
```

## Install

```bash
# Copy binaries
sudo install -m 0755 easywall-core /usr/sbin/easywall-core
sudo install -m 0755 easywall-web  /usr/sbin/easywall-web

# Create service user
sudo useradd --system --no-create-home --shell /usr/sbin/nologin easywall

# Create config and data directories
sudo mkdir -p /etc/easywall /var/lib/easywall /var/log/easywall /run/easywall

# Write default configs
sudo easywall-core --write-config /etc/easywall/easywall.toml
sudo easywall-web  --write-config /etc/easywall/web.toml

# Set permissions
sudo chown -R root:easywall /etc/easywall
sudo chmod 0750 /etc/easywall
sudo chmod 0600 /etc/easywall/easywall.toml /etc/easywall/web.toml
sudo chown easywall:easywall /var/lib/easywall /var/log/easywall
```

## systemd Services

```bash
sudo cp systemd/easywall-core.service /etc/systemd/system/
sudo cp systemd/easywall-web.service  /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now easywall-core easywall-web
```

## First Run

Open `https://<your-server>:12227` in your browser. Accept the self-signed certificate warning and complete the first-run wizard to set a username and password.
