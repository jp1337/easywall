---
layout: default
title: Debian / Ubuntu
description: The .deb package — two systemd units, config in /etc/easywall, done.
---

# Debian / Ubuntu

```bash
wget https://github.com/jp1337/easywall/releases/latest/download/easywall_amd64.deb
sudo dpkg -i easywall_amd64.deb && sudo apt-get install -f
```

Then open `https://<server>:12227`. The certificate is self-signed on first start, so
the browser will warn — accept it, or [configure your own](#your-own-certificate).
The first visit runs the setup wizard.

`arm64` packages are on the same release page.

## Where everything lives

| Path | |
|---|---|
| `/etc/easywall/easywall.toml` | core: firewall options, acceptance window, Docker |
| `/etc/easywall/web.toml` | web: auth, TLS, language, bind address |
| `/var/lib/easywall/rules.json` | the three rule sets |
| `/var/log/easywall/audit.log` | [audit log]({{ '/features/audit-log/' | relative_url }}), rotated daily |

Full key reference: [Configuration]({{ '/configuration/' | relative_url }}).

## Running it

```bash
systemctl status  easywall-core easywall-web
journalctl -u easywall-core -f
journalctl -u easywall-web  -f
systemctl restart easywall-core easywall-web
```

`easywall-core` runs as root; `easywall-web` runs as the unprivileged `easywall`
user. Both start on boot.

## Your own certificate

```toml
# /etc/easywall/web.toml
[tls]
cert = "/etc/letsencrypt/live/example.com/fullchain.pem"
key  = "/etc/letsencrypt/live/example.com/privkey.pem"
```

```bash
systemctl restart easywall-web
```

The `easywall` user must be able to read both files — a Let's Encrypt `privkey.pem`
is `root:root 0600` by default.

## Removing it

```bash
sudo apt remove easywall          # keeps /etc/easywall and the rules
sudo apt purge  easywall          # removes them too
```

Purging does **not** flush the rules from the kernel. Do that first if you want the
host open again:

```bash
sudo nft delete table inet easywall
```
