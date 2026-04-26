# easywall

[![Build](https://github.com/jpylypiw/easywall/actions/workflows/test.yml/badge.svg)](https://github.com/jpylypiw/easywall/actions)
[![Security](https://github.com/jpylypiw/easywall/actions/workflows/security.yml/badge.svg)](https://github.com/jpylypiw/easywall/actions)
[![License: GPL v3](https://img.shields.io/badge/License-GPLv3-blue.svg)](https://www.gnu.org/licenses/gpl-3.0)
[![Go Version](https://img.shields.io/badge/Go-1.21+-00ADD8.svg)](https://go.dev)

**Linux firewall management with a web interface.** Built for 2026: Go, nftables, security-first.

---

## Why easywall in 2026?

Linux servers and desktops remain high-value targets. Many hosting providers
offer no upstream firewall. The Linux desktop market share is growing, developer
laptops run on Linux, and simple, trustworthy firewall tools are scarce.

The original easywall (Python/Flask/iptables, v0.3.1) was archived after a CVE.
This complete rewrite addresses the root causes: no subprocess execution,
no privilege escalation from the web process, Argon2id authentication,
and nftables via direct netlink.

## Architecture

```
Browser  ──HTTPS──►  easywall-web  (user: easywall, unprivileged)
                           │
                    Unix socket (mode 0660, group easywall)
                    Typed JSON protocol
                           │
                     easywall-core  (root, CAP_NET_ADMIN only)
                           │
                    nftables kernel (via direct netlink — no nft subprocess)
```

## Features

- **nftables backend** — direct netlink API, no subprocess, no injection risk
- **Two-step activation** — rules auto-roll back if SSH is not confirmed
- **Docker coexistence** — own table `inet easywall`, never touches Docker chains
- **Protection modules** — SSH brute-force, SYN/ICMP flood, port scan, bogon filter, and more
- **i18n** — English & German, easily extensible
- **Light/Dark mode** — OS preference detection + manual toggle
- **Export/Import** — JSON rule backups
- **Single binary** — no Python, no pip, no runtime dependencies

## Quick Start

**Debian / Ubuntu:**
```bash
wget https://github.com/jpylypiw/easywall/releases/latest/download/easywall_amd64.deb
sudo dpkg -i easywall_amd64.deb && sudo apt-get install -f
xdg-open https://localhost:12227
```

**Docker:**
```bash
git clone https://github.com/jpylypiw/easywall.git && cd easywall
docker compose up -d
xdg-open https://localhost:12227
```

## Documentation

Full documentation at **[jpylypiw.github.io/easywall](https://jpylypiw.github.io/easywall)**

## Security

Please report vulnerabilities via
[GitHub Security Advisories](https://github.com/jpylypiw/easywall/security/advisories/new)
— not as public issues. See [SECURITY.md](SECURITY.md).

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). We use Conventional Commits.

## License

GPL-3.0 — see [LICENSE](LICENSE).
