---
layout: default
title: Second Factor
description: TOTP and eight one-time recovery codes for the single account — and the way back if you lose both.
---

# Second Factor

A stolen password alone no longer opens the firewall.

## Setting it up

On the **Password** page, under **Second factor**:

| Step | What happens |
|---|---|
| 1 | Enter your current password and start setup. A QR code and a typed key appear — nothing is saved yet. |
| 2 | Scan the QR code with an authenticator app, or type the key in by hand. |
| 3 | Enter the six-digit code the app shows. Only now is the secret written, together with eight recovery codes. |

The page also shows the server's own clock, in words, next to the QR code. If
the code you type is right but does not match the current time step, that is
the diagnosis: the app and the server disagree about the time, not about the
secret. The page says which way and by roughly how much, because a phone with
the correct time and a server with the wrong one looks, from the six digits
alone, exactly like a mistyped key — and fixing the wrong thing is how people
lock themselves out of their own firewall.

The eight recovery codes are shown once, on the same page, right after the
setup succeeds — **these will not be shown again**. There is no page or link
that can produce them a second time; only losing the phone and using one, or
generating a fresh set, brings the count back into view. Reloading the page
afterwards shows that a factor is enrolled, and how many of the eight are
left, and nothing more.

## Signing in with it

| Step | What happens |
|---|---|
| 1 | Username and password, as before |
| 2 | Six digits from your authenticator app — **or** one of the eight recovery codes, in the same field |

Three wrong codes and you are back at the sign-in form. Nothing tells you whether
the password or the code was the problem, and nothing counts down for you.

The password step ends in that second screen and nowhere else: with a factor
enrolled, entering the right password issues no session at all, only an
intermediate state bound to the credentials it was issued under. Changing the
password ends any half-login still waiting at the second step, the same way it
ends a finished one.

## If you lose the phone

Use a recovery code. It signs you in once and is then gone; the interface says
how many are left, and **Password → Second factor → New codes** issues eight
fresh ones and invalidates every old one.

## If you lose both

Edit `web.toml` on the host — the same file the password lives in:

```toml
totp_secret    = ""
recovery_codes = []
```

Restart `easywall-web`. The password alone signs you in again. There is no reset
link: this interface sends no mail and reaches no outside service.

---

**Next:** [Security]({{ '/docs/security/' | relative_url }}) ·
[First Run]({{ '/docs/installation/first-run/' | relative_url }}) ·
[Audit Log]({{ '/docs/features/audit-log/' | relative_url }})
