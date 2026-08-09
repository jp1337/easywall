---
layout: default
title: Manual Installation
description: Build the two binaries and install them yourself, when there is no package for your distribution.
---

# Manual Installation

For distributions without a package. Check
[Requirements]({{ '/installation/requirements/' | relative_url }}) first.

## Build

Go 1.25+ on the **build** machine — not on the target.

```bash
git clone https://github.com/jp1337/easywall.git
cd easywall
make build          # produces bin/easywall-core and bin/easywall-web
```

The version from `git describe` is embedded at build time. Cross-compiling works the
usual way:

```bash
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 make build
```

## Install

```bash
sudo make install
```

That places the binaries in `/usr/sbin`, the assets in `/usr/share/easywall`, and both
systemd units in `/lib/systemd/system`. It does **not** create the service user, the
directories or the config — do that once:

```bash
sudo groupadd --system easywall 2>/dev/null || true
sudo useradd  --system --no-create-home --shell /usr/sbin/nologin \
              --gid easywall easywall 2>/dev/null || true

sudo install -d -m 0750 -o root     -g easywall /run/easywall
sudo install -d -m 0750 -o root     -g easywall /etc/easywall
sudo install -d -m 0750 -o easywall -g easywall /etc/easywall/ssl
sudo install -d -m 0750 -o easywall -g easywall /var/lib/easywall
sudo install -d -m 0750 -o root     -g easywall /var/log/easywall

sudo install -m 0600 -o root -g root config/easywall.toml /etc/easywall/
```

The ownership is the point, not a detail. `easywall-web` runs as the `easywall`
user, so it needs to traverse `/etc/easywall` and read and rewrite its own
`web.toml` — the first-run wizard writes the password hash there. It must **not**
be able to write `easywall.toml`, which is the configuration the root daemon
loads: a network-facing process that can change what root reads has undone the
two-process split. Hence root-owned directory, group `easywall` for traverse, and
one file each side of the line.

`ssl_dir` belongs to `easywall` because easywall generates its own certificate
there and replaces it before it expires.

## Configure

One secret is needed — the key that signs session cookies:

```bash
sudo tee /etc/easywall/web.toml > /dev/null <<EOF
bind_addr   = "0.0.0.0:12227"
socket_path = "/run/easywall/core.sock"
ssl_dir     = "/etc/easywall/ssl"
data_dir    = "/var/lib/easywall"
language    = "en"
session_key = "$(openssl rand -hex 32)"
username    = ""
password    = ""
[tls]
cert = ""
key  = ""
EOF

sudo chown easywall:easywall /etc/easywall/web.toml
sudo chmod 0600              /etc/easywall/web.toml
```

Leave `session_key` out and easywall generates one on first start and writes it
back — but generating it here is better, because then it exists before the port
is ever open.

> **No `csrf_key` is needed.** CSRF protection is Go 1.25's
> `net/http.CrossOriginProtection`, which checks `Origin` and `Sec-Fetch-Site` instead
> of issuing tokens. The key that older configs and older versions of this page asked
> for is not read by anything.

## Start

```bash
sudo systemctl enable --now easywall-core easywall-web
systemctl status easywall-core easywall-web     # both should be active (running)
```

Open `https://<server>:12227`, accept the self-signed certificate, and complete the
setup wizard.

## Uninstall

```bash
sudo systemctl disable --now easywall-core easywall-web
sudo rm -f /lib/systemd/system/easywall-core.service \
           /lib/systemd/system/easywall-web.service
sudo systemctl daemon-reload

sudo rm -f  /usr/sbin/easywall-core /usr/sbin/easywall-web
sudo rm -rf /usr/share/easywall

# the rules stay in the kernel until you say otherwise
sudo nft delete table inet easywall 2>/dev/null || true

# config, rules and logs — back them up first if you want them
sudo rm -rf /etc/easywall /var/lib/easywall /var/log/easywall
sudo userdel  easywall 2>/dev/null || true
sudo groupdel easywall 2>/dev/null || true
```
