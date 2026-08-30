---
layout: default
title: Debian / Ubuntu
description: The .deb package — two systemd units, config in /etc/easywall, done.
---

# Debian / Ubuntu

```bash
ARCH=$(dpkg --print-architecture)      # amd64 or arm64
wget https://github.com/jp1337/easywall/releases/latest/download/easywall_$ARCH.deb
sudo dpkg -i easywall_$ARCH.deb && sudo apt-get install -f
```

| Architecture | File | Typical host |
|---|---|---|
| `amd64` | `easywall_amd64.deb` | a VPS, a physical server |
| `arm64` | `easywall_arm64.deb` | a Raspberry Pi, an arm64 VPS, Apple-silicon VMs |

The name never carries a version — that lives inside the package, where
`dpkg -l easywall` and `apt policy easywall` read it from, so the command does not
need editing for each release.

Those two are the whole list. If `dpkg --print-architecture` says anything else —
`armhf` on a 32-bit Raspberry Pi image, for instance — there is no package for it and
the download 404s; build [from source]({{ '/docs/installation/manual/' | relative_url }})
instead.

Then open `https://<server>:12227` and complete the
[setup]({{ '/docs/installation/first-run/' | relative_url }}). The certificate is
self-signed on first start, so the browser will warn — accept it, or
[configure your own](#your-own-certificate).

Both packages are built on a runner of their **own** architecture, never
cross-compiled, and each is installed and started on that architecture in CI before
it is published. The release refuses to publish a package whose `Architecture` field
or whose binaries do not match the leg that built it.

> **Every release before 2.5.0 was missing this file.** No release carried a `.deb`
> at all. It was a CI artefact that expired after seven days and needed a GitHub
> login, so the documented install path never worked for anyone. From 2.5.1 the
> release publishes it and refuses to publish one without binaries in it.

## Where everything lives

| Path | |
|---|---|
| `/etc/easywall/easywall.toml` | core: firewall options, acceptance window, Docker |
| `/etc/easywall/web.toml` | web: auth, TLS, language, bind address |
| `/var/lib/easywall/rules.json` | the three rule sets |
| `/var/log/easywall/audit.log` | [audit log]({{ '/docs/features/audit-log/' | relative_url }}), rotated daily |
| `/etc/easywall/*.toml.template` | the commented defaults the package carries |

Full key reference: [Configuration]({{ '/docs/configuration/' | relative_url }}).

**Upgrades never touch your two `.toml` files.** The package installs the
templates and creates each real file only when it is missing. So neither is a
dpkg conffile, and an upgrade cannot prompt about one or overwrite it. easywall
edits both itself. The settings pages write `easywall.toml`. The wizard and the
password page write `web.toml`. A file a program rewrites has no business
being managed by the package manager. The templates are also where to look for a
key a newer release added: they are replaced on upgrade, your files are not.

To start over from a default, delete the file and reinstall the package, or copy
the template yourself:

```bash
sudo cp /etc/easywall/easywall.toml.template /etc/easywall/easywall.toml
sudo chown root:root /etc/easywall/easywall.toml && sudo chmod 600 /etc/easywall/easywall.toml
sudo systemctl restart easywall-core
```

## Running it

```bash
systemctl status  easywall-core easywall-web
journalctl -u easywall-core -f
journalctl -u easywall-web  -f
systemctl restart easywall-core easywall-web
```

`easywall-core` runs as root; `easywall-web` runs as the unprivileged `easywall`
user. Both start on boot, and the core now puts the last confirmed rules back
into the kernel when it does — see
[Recovery & Panic Mode]({{ '/docs/features/recovery/' | relative_url }}).

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
