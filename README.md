<p align="center">
  <img src="web/static/icon.svg" alt="easywall logo" width="96" height="96">
</p>

# 🔥 easywall

[![Build](https://github.com/jp1337/easywall/actions/workflows/test.yml/badge.svg)](https://github.com/jp1337/easywall/actions/workflows/test.yml)
[![Security](https://github.com/jp1337/easywall/actions/workflows/security.yml/badge.svg)](https://github.com/jp1337/easywall/actions/workflows/security.yml)
[![codecov](https://codecov.io/gh/jp1337/easywall/graph/badge.svg)](https://codecov.io/gh/jp1337/easywall)
[![License: GPL v3](https://img.shields.io/badge/license-GPL--3.0-blue?logo=opensourceinitiative&logoColor=white)](https://www.gnu.org/licenses/gpl-3.0)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![nftables](https://img.shields.io/badge/nftables-direct%20netlink-informational?logo=linux&logoColor=white)](https://netfilter.org/projects/nftables/)
[![GitHub Sponsors](https://img.shields.io/badge/sponsor-GitHub-ea4aaa?logo=github-sponsors&logoColor=white)](https://github.com/sponsors/jp1337)
[![Ko-fi](https://img.shields.io/badge/support-Ko--fi-ff5e5b?logo=ko-fi&logoColor=white)](https://ko-fi.com/jpylypiw)
[![PayPal](https://img.shields.io/badge/donate-PayPal-003087?logo=paypal&logoColor=white)](https://paypal.me/JPylypiw)

> *Your firewall. Your rules. No surprises.*

**Linux firewall management with a web interface — built for 2026.**

A complete rewrite of the original easywall (Python/Flask/iptables, archived after a CVE). New architecture: Go, nftables via direct netlink, two-process isolation, Argon2id auth — security problems addressed at the root.

📖 **Documentation:** [jp1337.github.io/easywall](https://jp1337.github.io/easywall)

---

## 🏗️ Architecture

```
Browser  ──HTTPS──►  easywall-web   (user: easywall, unprivileged)
                           │
                    Unix socket (mode 0660, group easywall)
                    Typed JSON protocol
                           │
                     easywall-core  (root, CAP_NET_ADMIN only)
                           │
                    nftables kernel (via direct netlink — no nft subprocess)
```

The web process **never touches the firewall directly**. All changes go through a typed socket protocol to a privileged core daemon — privilege escalation from the web process is structurally impossible.

---

## ✨ Features

- **nftables backend** — direct netlink API via `google/nftables`, no subprocess, no shell, no injection risk
- **Two-step activation** — apply rules, then confirm over SSH within a configurable window; auto-rollback on timeout
- **Docker coexistence** — own table `inet easywall`, never touches Docker's chains; auto-detects bridge networks
- **TCP/UDP port management** — with descriptions and SSH brute-force routing per rule
- **IP blacklist & whitelist** — IPv4/IPv6 CIDRs, applied before any other rules
- **Port forwarding** — NAT rules with protocol selection
- **Custom rules** — raw nftables syntax, validated before apply
- **Export / Import** — full JSON rule backups, downloadable and re-uploadable
- **i18n** — English & German, extensible via `locales/<lang>.json`
- **Light / Dark mode** — follows OS preference, manual toggle available

### 🛡️ Protection Modules

| Module | Default | Description |
|---|---|---|
| SSH brute-force | ✅ on | Connection limit per source IP |
| ICMP flood | ✅ on | Rate-limit per source IP |
| SYN flood | ✅ on | Rate-limit new TCP connections |
| Port scan | ✅ on | Drops NULL, FIN, XMAS, SYN+FIN probes |
| Invalid packets | ✅ on | `ct state invalid` → DROP |
| IP fragments | off | Drop fragmented packets |
| Bogon filter | off | RFC-1918 from external interface → DROP |
| Connection limit | off | Max simultaneous connections per source IP |
| TCP RST flood | off | Rate-limit RST packets |
| Broadcast drop | off | `pkttype broadcast` → DROP |
| Multicast drop | off | `pkttype multicast` → DROP |

---

## 🛠️ Tech Stack

| Component | Choice | Notes |
|---|---|---|
| **Language** | Go 1.25 | Single-binary, no runtime dependencies |
| **HTTP router** | `go-chi/chi/v5` | Lightweight, idiomatic middleware chain |
| **Templates** | `html/template` (stdlib) | Auto-escaping — XSS structurally prevented |
| **nftables** | `google/nftables` | Direct netlink — no `nft` subprocess |
| **Password hashing** | `golang.org/x/crypto` Argon2id | Memory-hard, resistant to GPU cracking |
| **Sessions** | `gorilla/sessions` | HMAC-signed cookies, 600s lifetime |
| **CSRF** | `net/http.CrossOriginProtection` | Go 1.25 native, no form tokens needed |
| **Rate limiting** | `golang.org/x/time/rate` | Token bucket, per-IP on `/login` |
| **i18n** | `go-i18n/v2` | JSON message files |
| **Config** | `BurntSushi/toml` + JSON Schema | `taplo.toml` for editor autocomplete |
| **Security scan** | `govulncheck` + `gosec` | CVE + security linter in CI |

---

## 🚀 Quick Start

### Debian / Ubuntu

```bash
wget https://github.com/jp1337/easywall/releases/latest/download/easywall_amd64.deb
sudo dpkg -i easywall_amd64.deb && sudo apt-get install -f
xdg-open https://localhost:12227
```

### Docker

```bash
git clone https://github.com/jp1337/easywall.git
cd easywall
docker compose up -d
xdg-open https://localhost:12227
```

### Manual (from source)

#### 1. Prerequisites

- Linux kernel ≥ 3.13 with nftables (`apt install nftables`)
- Go 1.25+

#### 2. Build

```bash
git clone https://github.com/jp1337/easywall.git
cd easywall
make build
# Produces: bin/easywall-core  bin/easywall-web
```

#### 3. Install

```bash
sudo make install
sudo systemctl enable --now easywall-core easywall-web
xdg-open https://localhost:12227
```

The first visit opens the **setup wizard** to set your username and password.

---

## 📖 Documentation

Full documentation at **[jp1337.github.io/easywall](https://jp1337.github.io/easywall)**

| Guide | Description |
|---|---|
| [Requirements](https://jp1337.github.io/easywall/installation/requirements/) | Kernel version, distro compatibility matrix |
| [Debian / Ubuntu](https://jp1337.github.io/easywall/installation/debian/) | `.deb` package install |
| [Docker](https://jp1337.github.io/easywall/installation/docker/) | Docker Compose setup, `network_mode: host` |
| [Manual](https://jp1337.github.io/easywall/installation/manual/) | Build from source |
| [Configuration](https://jp1337.github.io/easywall/configuration/) | All TOML keys explained, JSON Schema |
| [Firewall Filters](https://jp1337.github.io/easywall/features/filters/) | Protection modules in detail |
| [Docker Coexistence](https://jp1337.github.io/easywall/features/docker/) | How easywall and Docker live together |
| [Export & Import](https://jp1337.github.io/easywall/features/export-import/) | JSON rule backups |
| [Security Model](https://jp1337.github.io/easywall/security/) | Two-process isolation, CVE history |

---

## 🔐 Security

easywall takes a **layered security approach** — each layer independently limits blast radius:

| Threat | Mitigation |
|---|---|
| Rule/command injection | Direct netlink API (no subprocess, no string-building) + typed Go structs |
| Privilege escalation | Web process runs as unprivileged `easywall` user — no root access |
| Auth brute-force | Rate-limiting on `/login` (5 req / 10 min per IP), Argon2id |
| CSRF | `net/http.CrossOriginProtection` (Go 1.25 native) |
| XSS | `html/template` auto-escaping + `Content-Security-Policy` header |
| Session hijacking | HTTPS-only cookie, `SameSite=Lax` |
| Lockout | Two-step activation with auto-rollback — bad rules can't lock you out permanently |
| Known CVEs | `govulncheck` in CI (weekly + every PR) |

Report vulnerabilities via [GitHub Security Advisories](https://github.com/jp1337/easywall/security/advisories/new) — not as public issues. See [SECURITY.md](SECURITY.md).

---

## 📦 Project Status

| Phase | Status | Description |
|---|---|---|
| Phase 1 — Foundation | ✅ Done | Go module, shared types, IPC protocol, version check |
| Phase 2 — Core Daemon | ✅ Done | nftables backend, rules storage, acceptance, Docker coexistence |
| Phase 3 — Web Backend | ✅ Done | chi router, Argon2id auth, session management, all handlers |
| Phase 4 — Web Frontend | ✅ Done | Templates, CSS custom properties, HTMX, light/dark mode |
| Phase 5 — Deployment | ✅ Done | systemd units, Docker multi-stage, Debian package |
| Phase 6 — Documentation | ✅ Done | MkDocs Material, GitHub Pages, custom theme |
| Phase 7 — CI/CD | ✅ Done | Test, Security, Build, Release, Docs workflows |

### Roadmap

| Feature | Notes |
|---|---|
| 2FA / TOTP | Second factor for the web UI |
| Let's Encrypt ACME | Automatic TLS certificates without a reverse proxy |
| GeoIP blocking | Country-based rules (requires GeoIP database) |
| REST API | For Ansible and automation integrations |

---

## 🤝 Contributing

easywall is open source and welcomes contributions. See [CONTRIBUTING.md](CONTRIBUTING.md) for setup, commit conventions (Conventional Commits), and the PR process.

---

## 📜 License

GPL-3.0 — see [LICENSE](LICENSE) for details.

---

*A rewrite that treats the root causes, not the symptoms.*
