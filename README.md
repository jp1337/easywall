<p align="center">
  <img src="web/static/icon.svg" alt="" width="88" height="88">
</p>

<h1 align="center">easywall</h1>

<p align="center"><em>Your firewall. Your rules. No surprises.</em></p>

<p align="center">
  <a href="https://github.com/jp1337/easywall/actions/workflows/test.yml"><img src="https://github.com/jp1337/easywall/actions/workflows/test.yml/badge.svg" alt="Build"></a>
  <a href="https://github.com/jp1337/easywall/actions/workflows/security.yml"><img src="https://github.com/jp1337/easywall/actions/workflows/security.yml/badge.svg" alt="Security"></a>
  <a href="https://codecov.io/gh/jp1337/easywall"><img src="https://codecov.io/gh/jp1337/easywall/graph/badge.svg" alt="Coverage"></a>
  <a href="https://www.gnu.org/licenses/gpl-3.0"><img src="https://img.shields.io/badge/license-GPL--3.0-blue?logo=opensourceinitiative&logoColor=white" alt="GPL-3.0"></a>
  <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white" alt="Go 1.25+"></a>
</p>

<p align="center">
  <a href="https://easywall.wdkro.de"><strong>Live demo</strong></a> ·
  <a href="https://jp1337.github.io/easywall"><strong>Documentation</strong></a> ·
  <a href="CHANGELOG.md">Changelog</a>
</p>

nftables through a web interface that cannot lock you out: **every apply reverts
itself unless you confirm it.**

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/img/screens/dashboard-dark.png">
  <img src="docs/assets/img/screens/dashboard-light.png" alt="The easywall dashboard: firewall status with acceptance state, pending changes and last apply; tiles counting TCP ports, UDP ports, blacklist, whitelist, custom rules and forwarding; and a recent-activity list.">
</picture>

## The idea

Editing a rule changes nothing. Applying it changes everything — for 120 seconds.
If the new rules cut your connection you cannot click Confirm, and *not* confirming
is what brings the old rules back.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/diagrams/apply-flow-dark.svg">
  <img src="docs/assets/diagrams/apply-flow-light.svg" alt="State machine: editing leads to Staged, applying leads to Live, confirming within the window leads to Confirmed, and letting the window expire leads to Rolled back, from where the staged edits are still available.">
</picture>

## Architecture

Two processes. The one exposed to the network holds no privilege worth stealing.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/diagrams/architecture-dark.svg">
  <img src="docs/assets/diagrams/architecture-light.svg" alt="Browser talks HTTPS to easywall-web, which runs unprivileged; easywall-web talks typed JSON over a Unix socket to easywall-core, which runs as root and speaks netlink to the nftables table inet easywall.">
</picture>

A complete rewrite of the original easywall — Python, Flask, `iptables` via
subprocess — which was archived in 2022 after a CVE. Both root causes are gone:
the privileges live in a different process, and the apply path builds Go structs
instead of a command line. [How it works →](https://jp1337.github.io/easywall/architecture/)

## Install

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/diagrams/install-choice-dark.svg">
  <img src="docs/assets/diagrams/install-choice-light.svg" alt="Decision tree: just looking leads to demo mode; Debian or Ubuntu leads to the .deb package; already running containers leads to Docker; otherwise build from source.">
</picture>

```bash
# Debian / Ubuntu
wget https://github.com/jp1337/easywall/releases/latest/download/easywall_amd64.deb
sudo dpkg -i easywall_amd64.deb && sudo apt-get install -f

# Docker
git clone https://github.com/jp1337/easywall.git && cd easywall && docker compose up -d

# From source — Go 1.25+, nftables
git clone https://github.com/jp1337/easywall.git && cd easywall
make build && sudo make install
sudo systemctl enable --now easywall-core easywall-web
```

Then open `https://localhost:12227`. The first visit runs the setup wizard.

## What you get

| | |
|---|---|
| **Ports** | TCP and UDP, single or range, with per-rule SSH brute-force routing |
| **Blacklist & whitelist** | IPv4, IPv6 and CIDR, evaluated before any port rule |
| **Protection modules** | Nine, four on by default — floods, scans, bogons, fragments |
| **Port forwarding** | NAT redirects with protocol selection |
| **Custom rules** | Raw nftables, syntax-checked before it is applied |
| **Export / import** | The whole rule set as JSON |
| **Audit log** | Who changed what, when |
| **Docker coexistence** | Owns `table inet easywall`, touches nothing else |
| **English & German** | Switchable in the interface, including before sign-in |
| **Light & dark** | Follows the OS, with a manual toggle; both contrast-checked |

## Built with

| | |
|---|---|
| Go 1.25, single binary | `go-chi/chi` · `html/template` |
| nftables via `google/nftables` | direct netlink, no `nft` subprocess |
| Argon2id | `golang.org/x/crypto`, 16-byte salt per password |
| CSRF | `net/http.CrossOriginProtection`, Go 1.25 native |
| Design system | [`DESIGN.md`](DESIGN.md) + Tailwind v4 — no third-party UI library |
| Fonts | Inter + JetBrains Mono, self-hosted, ~145 KB — works air-gapped |
| CI | `govulncheck`, `gosec`, CodeQL, 90% web coverage |

## Roadmap

| | |
|---|---|
| 2FA / TOTP | Second factor for the web interface |
| Let's Encrypt ACME | Certificates without a reverse proxy |
| Audit log for logins | Authentication events are not recorded yet |
| REST API | For Ansible and automation |

## Contributing

Setup, commit conventions and the review checklist: [CONTRIBUTING.md](CONTRIBUTING.md).
Anything visual goes through [`DESIGN.md`](DESIGN.md) first.

Security issues: **not** as a public issue — use
[GitHub Security Advisories](https://github.com/jp1337/easywall/security/advisories/new).

## License

GPL-3.0 — see [LICENSE](LICENSE).
