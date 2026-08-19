---
layout: default
title: Second Factor
description: TOTP and eight one-time recovery codes for the single account — and the way back if you lose both.
---

# Second Factor

A stolen password alone no longer opens the firewall.

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
