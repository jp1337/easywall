---
layout: default
title: Manual Installation
description: Build the two binaries and install them yourself, when there is no package for your distribution.
---

# Manual Installation

For distributions without a package. Check
[Requirements]({{ '/installation/requirements/' | relative_url }}) first.

## Build

Go 1.26+ on the **build** machine — not on the target. That is the toolchain
`go.mod` pins; on Debian trixie it comes from backports, since trixie itself ships
Go 1.24.

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
sudo install -d -m 0770 -o root     -g easywall /var/lib/easywall
sudo install -d -m 0750 -o root     -g easywall /var/log/easywall

sudo install -m 0600 -o root -g root config/easywall.toml /etc/easywall/
```

**The ownership is the point, not a detail.**

| Path | Why that owner and mode |
|---|---|
| `/etc/easywall` — `root:easywall` `0750` | the web user must **traverse** it to reach its own config, and must not write in it |
| `easywall.toml` — `root:root` `0600` | the root daemon reads this. A network-facing process that can change what root reads has undone the two-process split |
| `web.toml` — `easywall:easywall` `0600` | the wizard writes the password hash here, and the password page rewrites it |
| `ssl_dir` — `easywall:easywall` `0750` | easywall generates its own certificate there and replaces it before it expires |
| `/var/lib/easywall` — `root:easywall` `0770` | **shared**: the root core writes `rules.json`, the web user writes its caches. Group-writable so the core does not need `CAP_DAC_OVERRIDE` |
| `/run/easywall` — `root:easywall` `0750` | holds the control socket the web process connects to |

> **`/var/lib/easywall` used to be listed here as `easywall:easywall`, and that does
> not work.** The unit reduces the daemon to `CAP_NET_ADMIN`, which for a root
> service also removes `CAP_DAC_OVERRIDE` — so root cannot enter a directory it does
> not own, `rules.json` is never written, and every request fails on a permission
> error raised deep inside the call. The package had the same bug and it is what the
> `0770 root:easywall` above fixes.

## Configure

Both binaries carry their own commented default and will write it out. Neither
overwrites an existing file.

```bash
sudo easywall-web  --write-config /etc/easywall/web.toml
sudo chown easywall:easywall      /etc/easywall/web.toml

sudo easywall-core --write-config /etc/easywall/easywall.toml
```

Both land as `0600`; the `chown` is what makes `web.toml` writable by the process
that has to rewrite it. Every key is explained in
[Configuration]({{ '/configuration/' | relative_url }}).

The session key ships as an obvious placeholder and easywall replaces it on first
start. Generating it yourself is better — then it exists before the port is ever
open:

```bash
sudo sed -i "s|CHANGE_ME[A-Z0-9_]*|$(openssl rand -hex 32)|" /etc/easywall/web.toml
```

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
