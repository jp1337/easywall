# The socket protocol

`easywall-web` reaches `easywall-core` over a Unix socket at
`/run/easywall/core.sock`, `root:easywall 0660`. One JSON object per request, one
per reply, connection closed after. Declared as Go structs on both sides in
`internal/shared/protocol.go`; adding an operation means adding a constant to both
ends.

Seventeen command types:

| | |
|---|---|
| `GET_RULES` · `SAVE_RULES` | read all three sets · write **Staged** |
| `APPLY_RULES` · `ACCEPT` | promote Staged and start the timer · confirm |
| `GET_OPTIONS` · `SAVE_OPTIONS` | the protection modules |
| `GET_SETTINGS` · `SAVE_SETTINGS` | IPv6, Docker, routing |
| `GET_SYSTEM` · `SAVE_SYSTEM` | the acceptance window |
| `GET_STATUS` | dashboard state, asked of the kernel |
| `GET_LOG` | the last 200 audit entries |
| `EXPORT_RULES` · `IMPORT_RULES` | the rule set as JSON |
| `VALIDATE_CUSTOM` | `nft --check` for the live editor |
| `PANIC` · `RESUME` | tear the table down and record it as deliberate · end that and restore |

## The one field that is not typed

`SaveRulesPayload.Rules` is an `interface{}`. The core re-encodes it to JSON and
decodes it into the concrete type named by `rule_type`.

An unknown `rule_type` is rejected, and since 2.5.0 the decoded rules are
validated before they are stored — but the field itself is untyped at the protocol
level, and `docs/architecture.md` used to claim the whole protocol was typed. It is
the one place where "a JSON command the typed protocol accepts" means slightly less
than it sounds.

## What the protocol does not carry

**Identity.** There is no user field on any command, so every audit entry is
written with the literal `"web"` — `daemon.go` passes it at each call site. The
documentation must not imply otherwise: the audit log's `user` column answers
"which process", not "which person". Multiple accounts and a `user` field are the
2.7 line on the roadmap, in that order, because the field has to exist before
login events mean anything.

**A session.** Each request is independent. The core does not know which browser
caused it, which is also why rate limiting lives entirely in the web process.

## Concurrency and the apply timer

An apply that is already running rejects a second one with
`ErrApplyInProgressText`. The timer lives in the core, not the browser: closing the
tab, losing the connection or restarting the *web* process does not confirm
anything, and letting the window expire is what restores the previous set.

Stopping the **core** while a window is open counts as not confirming — it rolls
back before the daemon exits. A restart inside those two minutes is an ordinary
event, and it used to leave the unconfirmed set in place with nothing left to
undo it.

## Demo mode

`internal/web/democlient.go` implements the same client interface against an
in-memory state: every `Get*` returns it, every `Save*` updates it and appends an
audit entry, `APPLY_RULES` runs the real acceptance state machine.

`VALIDATE_CUSTOM` **reports the checker as unavailable** rather than answering. It
used to reply "no errors" whatever was typed — a false green on the one page where
being wrong locks you out.

Demo mode disables nothing else: authentication, CSRF, the CSP and the rate
limiter all behave normally, which is what makes it a usable target for the
Playwright suite in `test.yml`.

## Adding a command

1. The constant in `internal/shared/protocol.go`, plus its payload struct.
2. The handler in `internal/core/daemon.go` — and an audit entry if it changes
   anything.
3. The client method, and the demo client beside it. A demo client that silently
   does nothing is how the validation false-green happened.
4. If it changes the firewall: an integration test in `internal/core/`, because
   the unit tests mock the kernel and cannot tell a correct rule from a plausible
   one.
