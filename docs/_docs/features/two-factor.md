---
layout: default
title: Second Factor
description: TOTP and eight one-time recovery codes for the single account — and the way back if you lose both.
---

# Second Factor

A stolen password alone no longer opens the firewall.

## Switching it on

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/password" ext="png"
     alt="The Password page before a second factor is enrolled: the change-password form, and a Second factor card marked Off with a single button to start setup." %}
  <figcaption>Everyone starts here — Off, with one button. Nothing below this card exists until step 1 is entered.</figcaption>
</figure>

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/two-factor-setup" ext="png"
     alt="The second-factor setup card on the Password page: a QR code and a typed key on the left, a field for the six-digit confirmation code on the right, and the server's own clock printed beneath the QR code." %}
  <figcaption>Nothing is saved until the code in step 3 is entered — a phone that never scanned this screen leaves no trace.</figcaption>
</figure>

On the **Password** page, under **Second factor**:

| Step | What happens |
|---|---|
| 1 | Enter your current password and start setup. A QR code and a typed key appear — nothing is saved yet. |
| 2 | Scan the QR code with an authenticator app, or type the key in by hand. |
| 3 | Enter the six-digit code the app shows. Only now is the secret written, together with eight recovery codes. |

It can also be switched on during the first run, before this page exists to switch it on from — see [First Run]({{ '/docs/installation/first-run/' | relative_url }}).

## The eight codes

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/two-factor-codes" ext="png"
     alt="Eight one-time recovery codes shown once after setup succeeds, with a notice that they will not be shown again." %}
  <figcaption>This screen is the only time the eight codes are ever shown in full.</figcaption>
</figure>

The eight recovery codes are shown once, on the same page, right after the
setup succeeds — **these will not be shown again**. There is nothing to
download; write them down or put them in a password manager before leaving
this page. There is no page or link that can produce them a second time; only
losing the phone and using one, or generating a fresh set, brings the count
back into view. Reloading the page afterwards shows that a factor is
enrolled, and how many of the eight are left, and nothing more.

**Password → Second factor → New codes** issues eight fresh ones and
invalidates every old one, at any time — not only after losing the phone.

## Signing in

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/two-factor-verify" ext="png"
     alt="The second-step sign-in page: a single field for a six-digit code or a recovery code, with no indication of whether the earlier password was correct." %}
  <figcaption>Nothing on this screen says whether the password was right — only whether the code was.</figcaption>
</figure>

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

## The clock

The setup page shows the server's own clock, in words, next to the QR code —
not the sign-in page, deliberately. TOTP has no shared secret exchange at the
moment you type a code; it only works because both sides compute the same
thing from the same secret and the same 30-second time step. Setup is the one
moment an operator can still compare the phone's clock against the server's
before anything is committed, and it is also the moment a mismatch is
cheapest to fix — nothing is enrolled yet.

If the code you type at setup is right but does not match the current time
step, that is the diagnosis: the app and the server disagree about the time,
not about the secret. The page says which way and by roughly how much,
because a phone with the correct time and a server with the wrong one looks,
from the six digits alone, exactly like a mistyped key — and fixing the wrong
thing is how people lock themselves out of their own firewall.

## The way back

### If you lose the phone

Use a recovery code. It signs you in once and is then gone; the interface says
how many are left, and **Password → Second factor → New codes** issues eight
fresh ones and invalidates every old one.

### If you lose both

Edit `web.toml` on the host — the same file the password lives in:

```toml
totp_secret    = ""
recovery_codes = []
```

Restart `easywall-web`. The password alone signs you in again. There is no reset
link: this interface sends no mail and reaches no outside service.

Every step above — enrolling, signing in with a code, using a recovery code,
issuing new ones, a wrong code, or a factor switched off — is recorded. See
[the nine login events]({{ '/docs/features/audit-log/' | relative_url }}#the-nine-login-events).

---

**Next:** [Security]({{ '/docs/security/' | relative_url }}) ·
[First Run]({{ '/docs/installation/first-run/' | relative_url }}) ·
[Audit Log]({{ '/docs/features/audit-log/' | relative_url }})
