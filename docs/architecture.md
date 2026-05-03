---
layout: default
title: Architecture
description: How easywall works under the hood — two-process isolation, typed IPC, three-state rules, and the acceptance window. The structural reasons it cannot be exploited the way the original Python version was.
---

# Architecture

easywall is split into two processes with **structural privilege isolation**. Each process holds exactly the privileges it needs and nothing more. Privilege escalation from the web UI to the kernel firewall is not just blocked at runtime — it is *structurally impossible* because the privileges live in different processes that communicate only through a typed protocol over a Unix socket.

## The Two Processes

<div class="mermaid">
flowchart LR
    Browser["🌐 Browser"] -->|HTTPS :12227| Web
    Web["easywall-web<br/>unprivileged user"] -->|Unix socket<br/>typed JSON| Core
    Core["easywall-core<br/>root / CAP_NET_ADMIN"] -->|direct netlink<br/>no subprocess| NFT
    NFT["🐧 nftables<br/>Linux kernel"]
</div>

**`easywall-web`** runs as an unprivileged user. It serves the HTTPS UI, parses form input, talks to a session store, and renders templates. It cannot open any kernel sockets, read `/proc`, or execute `nft`. Its only capability is to send well-formed JSON over a Unix socket.

**`easywall-core`** runs as root (or with `CAP_NET_ADMIN`). It listens on the Unix socket, validates every command, and is the single point in the system that touches nftables — through the [`google/nftables`](https://pkg.go.dev/github.com/google/nftables) Go library, which speaks netlink directly. There is no `exec.Command("nft", …)` in the apply path.

Communication between them is a **typed JSON protocol**: every request is a `Command{Type, Payload}`, every response is a `Response{Success, Data, Error}`. Adding a new operation requires adding a new typed `CommandType` constant on both sides — there is no way to smuggle arbitrary commands across the boundary.

## Why Two Processes?

The original easywall (Python/Flask/iptables, archived in 2022 after a CVE) ran the entire stack as a single root process and shelled out to `iptables` via `subprocess.Popen`. Two consequences of that design caused the disclosed vulnerability:

1. **A bug in the web layer was a bug in the firewall.** Any flaw in form parsing, template rendering, or session handling could escalate to arbitrary firewall manipulation.
2. **Argument injection through subprocess.** User-supplied strings were passed to `iptables` as command-line arguments. A crafted input could inject arguments into the rule definition.

The Go rewrite addresses both root causes:

- **Privilege isolation by design.** A vulnerability in `easywall-web` cannot reach the kernel because the web process has no kernel access to misuse. The attacker has to additionally craft a JSON command that the typed protocol accepts and that the core's validation layer permits.
- **No subprocess in the apply path.** Rules are constructed as Go structs and sent to nftables via netlink. There is no shell to escape, no argument parser to confuse.

Custom rules are the one exception — they are passed to `nft -f -` because they are arbitrary nftables expressions that need the official parser. But that subprocess receives the input over stdin (no argv injection) and runs only inside the privileged core process. The web process never invokes a shell.

## Three-State Rule Storage

easywall stores rules in **three** parallel sets at all times:

```
RulesState {
    Current  // active in the kernel right now
    Staged   // editor changes the user is making
    Backup   // last-known-good for rollback
}
```

This separation is the foundation of the [acceptance window](#acceptance-window) safety mechanism and of the visible **"pending changes"** state on the dashboard. Edits land in `Staged` only; nothing reaches the kernel until the user explicitly clicks **Apply**.

## The Apply Cycle

When the user clicks **Apply**:

```
1. Backup ← Current      (so we can roll back)
2. Current ← Staged      (promote staged rules)
3. Apply Current to nftables via netlink
4. Mark acceptance: PENDING
5. Start the acceptance timer (default: 120s)
```

If the user clicks **Confirm** before the timer expires:

```
6a. Cancel timer
6b. Mark acceptance: ACCEPTED
6c. (briefly) → IDLE
```

If the timer expires without confirmation:

```
6a. Current ← Backup     (revert to last-known-good)
6b. Apply Current to nftables
6c. Mark acceptance: ROLLED_BACK
```

## Acceptance Window

The acceptance window is a deliberate safety mechanism for remote firewall management. A misconfigured rule set can lock the operator out of SSH, leaving the only recourse a console session, a serial cable, or a physical visit to the server.

With the acceptance window enabled, **applying rules is reversible by inaction**. If the new rules block your access, you simply do nothing — after `[acceptance].duration` seconds (10 to 3600), the core automatically reverts to the previous rule set. If your access still works, you click Confirm and the new rules become permanent.

This is the same pattern Cisco IOS uses for `commit confirmed` on routers and what experienced ops teams approximate manually with `at` jobs ("`at now + 5 minutes` `iptables -F`"). easywall makes it the default workflow.

See [System Settings]({{ '/features/system-settings/' | relative_url }}) for configuration options.

## The IPC Protocol

The Unix socket protocol is intentionally narrow. Each command is one of about 16 typed message kinds — the full list lives in [`internal/shared/protocol.go`](https://github.com/jp1337/easywall/blob/main/internal/shared/protocol.go) on GitHub:

| Command | Direction | Purpose |
|---------|-----------|---------|
| `GET_RULES` | web → core | fetch the three rule sets for the editor |
| `SAVE_RULES` | web → core | update `Staged` for one rule type |
| `APPLY_RULES` | web → core | promote `Staged` and start acceptance |
| `ACCEPT` | web → core | confirm pending rules |
| `GET_OPTIONS`, `SAVE_OPTIONS` | web ↔ core | firewall protection toggles |
| `GET_SETTINGS`, `SAVE_SETTINGS` | web ↔ core | IPv6 + Docker config |
| `GET_SYSTEM`, `SAVE_SYSTEM` | web ↔ core | acceptance window |
| `GET_LOG` | web → core | last 200 audit log entries |
| `GET_STATUS` | web → core | dashboard status |
| `EXPORT_RULES`, `IMPORT_RULES` | web ↔ core | backup/restore as JSON |
| `VALIDATE_CUSTOM` | web → core | run `nft --check` for live validation |

Every payload type is declared as a Go struct on both sides — there are no untyped fields, no map[string]interface{} pass-throughs.

## Threat Model Summary

What this architecture defends against:

- **Web-layer vulnerabilities** in form parsing, template rendering, session handling, etc. — they cannot reach the kernel firewall directly.
- **Argument injection** into `nft` — the apply path never exec's a shell.
- **CSRF** — Go 1.25's `net/http.CrossOriginProtection` rejects cross-origin POSTs based on `Origin` and `Sec-Fetch-Site` headers.
- **Lockout from misconfiguration** — the acceptance window auto-reverts.
- **Tampering with stored rule data** — the on-disk format is JSON with explicit schema; both processes validate.

What this architecture does **not** defend against:

- A compromised root account on the host.
- A vulnerability in the Linux kernel's nftables subsystem itself.
- Misuse by a legitimately authenticated administrator (use the [Audit Log]({{ '/features/audit-log/' | relative_url }}) to retrospectively detect this).

## Further Reading

- [Configuration]({{ '/configuration/' | relative_url }}) — every option in `easywall-core.toml` and `web.toml`
- [Security]({{ '/security/' | relative_url }}) — disclosed CVEs, hardening notes
- [Audit Log]({{ '/features/audit-log/' | relative_url }}) — what every administrative action records
- [Demo Mode]({{ '/installation/demo/' | relative_url }}) — single-binary in-memory deployment
