# 🔐 Security Policy

## Supported Versions

| Version | Supported |
|---|---|
| 2.x (current) | ✅ |
| 0.x (archived Python rewrite) | ❌ |

## Reporting a Vulnerability

**Do not open a public GitHub issue for security vulnerabilities.**

Use **[GitHub Security Advisories](https://github.com/jp1337/easywall/security/advisories/new)** for private, coordinated disclosure. This keeps the report confidential until a fix is ready.

### What to include

- Description of the vulnerability and its potential impact
- Steps to reproduce (proof of concept if possible)
- Affected version(s)
- Any suggested mitigations or patches

### Response timeline

| Milestone | Target |
|---|---|
| Initial acknowledgement | ≤ 48 hours |
| Confirmed / triaged | ≤ 7 days |
| Patch for critical issues | ≤ 14 days |
| Public disclosure | After patch is released |

### Recognition

Responsible disclosure is acknowledged in the release notes and in the security advisory. Thank you for helping keep easywall and its users safe.

---

## Security Architecture

easywall is designed with a layered approach — each layer independently limits the blast radius of a compromise:

- **No subprocess execution** — nftables rules are applied via direct netlink API (`google/nftables`), not through shell commands or the `nft` binary. Rule injection is structurally impossible.
- **Process isolation** — the web interface runs as an unprivileged user (`easywall`). Only the core daemon runs as root, and it only accepts typed JSON commands over a Unix socket.
- **Argon2id authentication** — resistant to GPU-based brute-force attacks.
- **CSRF protection** — via `net/http.CrossOriginProtection` (Go 1.25 native).
- **XSS prevention** — Go's `html/template` auto-escapes all output by default.
- **Two-step activation** — firewall changes require confirmation within a configurable window, preventing accidental lockouts.

See the full [Security Model](https://jp1337.github.io/easywall/security/) in the documentation.
