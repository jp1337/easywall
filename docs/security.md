---
layout: default
title: Security Model
description: easywall's layered security architecture — two-process isolation, Argon2id auth, nftables netlink API.
---

# Security Model

## Architecture Security

| Threat | Mitigation |
|---|---|
| Command/Rule Injection | Direct netlink API — no subprocess, no string interpolation into commands |
| Privilege Escalation | Web process runs as unprivileged `easywall` user |
| Auth Brute Force | Rate limiting (5 attempts / 10 min per IP), Argon2id hashing |
| CSRF | gorilla/csrf on all POST endpoints |
| XSS | Go `html/template` auto-escapes all output; Content-Security-Policy header |
| Session Hijacking | HTTPS-only, HttpOnly, SameSite=Lax session cookie |
| SSH Lockout | Two-step activation: rules auto-roll back if not confirmed within timeout |
| Audit Trail | `/var/log/easywall/audit.log` — structured JSON entries |
| Known CVEs | `govulncheck` in CI (weekly + on every PR) |
| Dependency Hijacking | Dependabot + GitHub Secret Scanning + Dependency Review |

## Process Isolation

<div class="mermaid">
flowchart TB
    Browser["🌐 Browser\nHTTPS :12227"]

    subgraph web["easywall-web  (user: easywall)"]
        W1["• No root privileges"]
        W2["• No direct kernel access"]
        W3["• Reads /etc/easywall (TLS, config)"]
        W4["• Binds 0.0.0.0:12227 (HTTPS)"]
    end

    subgraph core["easywall-core  (root)"]
        C1["• Owns core.sock (mode 0660)"]
        C2["• CAP_NET_ADMIN only"]
        C3["• Reads/writes /var/lib/easywall"]
        C4["• No network access"]
    end

    Kernel["🐧 nftables kernel\nvia direct netlink"]

    Browser -->|HTTPS| web
    web -->|"Unix socket\nTyped JSON protocol"| core
    core -->|"netlink syscalls\n(no nft subprocess)"| Kernel
</div>

## IPC Protocol Security

All communication between web and core uses typed Go structs serialised as JSON.
There are exactly 9 command types. The core daemon only accepts these known
command types — any unknown command returns an error without executing anything.

nftables rules are built programmatically from Go data structures via the
`github.com/google/nftables` netlink library. There is no string concatenation
into nftables syntax, eliminating rule injection by design.

## CVE History

The original easywall v0.3.1 (Python/Flask) received a CVE due to inadequate
input validation and privilege separation. v2.0 addresses the root cause:

- Web process no longer has root access
- No subprocess execution (no `iptables` or `nft` binary calls)
- No file-based IPC (replaced by typed socket protocol)
- No SHA512 with hostname salt (replaced by Argon2id)

## Reporting Security Issues

**Do not open a public GitHub issue for security vulnerabilities.**

Report via [GitHub Security Advisories](https://github.com/jp1337/easywall/security/advisories/new)
(private disclosure).

- Initial response: ≤ 48 hours
- Patch target for critical issues: ≤ 14 days
- Credit given for responsible disclosure
