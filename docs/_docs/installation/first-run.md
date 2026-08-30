---
layout: default
title: First Run
description: One page — the account, and how this host starts out. Everything but the account is staged.
---

# First Run

The first time you open `https://<server>:12227`, easywall serves the setup page
and nothing else. It asks for two things.

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/firstrun" ext="png"
     alt="The first-run page: an account section with username, password and confirmation on the left, and a first-choices section on the right with the SSH port, a note that the web port stays open, a switch for 80 and 443, three IPv6 options and a switch for counting the installation." %}
  <figcaption>The choices on the right are staged. Nothing reaches the firewall here.</figcaption>
</figure>

## Your account

| | |
|---|---|
| How many | one. easywall has no user management yet — [roadmap]({{ '/docs/roadmap/' | relative_url }}) |
| Password | at least 12 characters, hashed with Argon2id and a per-password salt |
| Recovery | **none by design.** No mail, no outside service — see [below](#if-you-lose-the-password) |

**The wizard offers a second factor, unticked by default.**

| Answer | What happens |
|---|---|
| Left unticked | the account is created with a password alone, same as before |
| Ticked | a setup step replaces Finish: the QR code, the typed key and the server's own clock, exactly as on [Second Factor]({{ '/docs/features/two-factor/' | relative_url }}) |
| On that step, confirmed | the six-digit code is checked; only then is the secret written, together with eight recovery codes shown once |
| On that step, skipped | the account is still created, with a password alone — skipping is a first-class answer here, not a failure |

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/firstrun-2fa" ext="png"
     alt="The first-run wizard's setup step: a QR code on a white plate, the typed key and the server's own clock on the left, a field for the six-digit confirmation code and a Confirm button on the right, and below them a Continue without a second factor button." %}
  <figcaption>Nothing is saved until the code is confirmed — and the escape hatch beneath it is never smaller than the button that saves something.</figcaption>
</figure>

You need an authenticator app already installed and in hand to confirm it on
this page. The first run is the moment an operator is least likely to have
one. It happens mid-installation, on a machine that may not even have a
browser tab to spare for scanning a QR code. Skipping costs nothing: a second
factor set up later works exactly the same way, reachable any time after
signing in on **Password → Second factor**: [Second Factor]({{ '/docs/features/two-factor/' | relative_url }}).

## First choices — all of them staged

| Answer | What it does |
|---|---|
| **SSH port** | staged as an open TCP port with brute-force protection ticked. Default 22 |
| **Port 12227** | added for you, because the firewall drops what it was not told to allow — including this page |
| **Also open 80 and 443** | two more staged ports, for a host serving a website |
| **IPv6** | filter it (almost always right), leave it alone, or drop it. Saved as a **setting**, not staged — it decides how every later rule is evaluated |
| **Count this installation** | off unless you switch it on. What it sends is printed in full under [Configuration]({{ '/docs/configuration/' | relative_url }}#counting-installations) |

**Staged means nothing is live yet.** After signing in, review it on
[Ports]({{ '/docs/features/ports/' | relative_url }}) and push it with
[Apply]({{ '/docs/features/apply/' | relative_url }}) — which still undoes itself unless
you confirm. The setup page is the worst possible moment to make an exception:
nobody has yet checked that they can still reach the machine.

**The SSH port is the one answer that can lock you out**, so it is checked
before the account is created. That happens while this page is still in front
of you, so you can correct it.

## What happens when you press Finish

**With the second factor left unticked:**

1. The account is written first. From that moment the setup page is closed and
   `/login` is served instead.
2. The choices are staged. If the core daemon is not answering, this is the part
   that fails — and it says so: *"Account created, but the choices could not be
   staged."* You can sign in and set them by hand.
3. You land on the sign-in page.

**With it ticked**, Finish does not write the account yet — it shows the setup
step instead, and the account is written only once that step is confirmed or
skipped. The choices are staged the same way either time, but the two endings
differ. **Confirming** lands you on the recovery codes, shown once and never
again. Sign-in is a deliberate click from there, not a redirect. **Skipping**
goes straight to the sign-in page, the same as leaving the box unticked.

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/firstrun-codes" ext="png"
     alt="Eight one-time recovery codes shown once, right after the first-run wizard confirms a second factor, with a Copy codes button and a Continue to sign in button." %}
  <figcaption>The only time these eight codes are shown. Copy them now; the second-factor page can issue new ones, but it cannot show these again.</figcaption>
</figure>

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/login" ext="png"
     alt="The easywall sign-in page: a card with username and password fields, a Sign in button, and a language menu in the footer." %}
  <figcaption>The language switch sits on the sign-in page too — an operator who cannot read the interface can still get in.</figcaption>
</figure>

If the wizard rejects something, every answer except the two passwords comes back
with the page. Retyping a password does not silently reset your SSH port to 22.

## Changing the password later

**System → Password** in the interface. You stay signed in on the device you
change it from. Every other session is refused immediately, because each
session carries a fingerprint of the password hash it was issued under.

### If you lose the password

There is no reset link. Recovery means shell access to the host:

```bash
sudo sed -i -E 's/^password[[:space:]]*=.*/password = ""/' /etc/easywall/web.toml
sudo systemctl restart easywall-web
```

Clearing the `password` line reopens this page. The rules, the audit log and every
setting are untouched — only the account is recreated.

## Behind a reverse proxy

easywall terminates TLS itself and does not trust `X-Forwarded-For`, deliberately:
a client that can set its own source address defeats the login rate limiter.

The consequence to know about: behind a proxy, **every sign-in attempt looks
like it comes from the proxy**. The limit of five attempts per ten minutes is
then shared by everyone. One person getting it wrong repeatedly locks the rest
out until the budget refills. Reaching easywall directly, or on a private
network, avoids it.

## When it does not work

| Symptom | Cause | Check |
|---|---|---|
| The setup page 404s | an account already exists | go to `/login`; clear the `password` line to start over |
| "That is not a port number" | the SSH port is outside 1–65535 | |
| "the choices could not be staged" | the core daemon was not reachable | `systemctl status easywall-core`, then set the ports by hand |
| The browser warns about the certificate | it is self-signed on first start | accept it, or [configure your own]({{ '/docs/installation/debian/' | relative_url }}#your-own-certificate) |
| Signed in, but every page says the core is unreachable | the socket is not reachable by the web user | `ls -l /run/easywall/core.sock` — it must be `root:easywall` |
| "Too Many Requests" on sign-in | five failed attempts in ten minutes from your address | wait; one attempt is returned every two minutes |

---

**Next:** [Applying rules]({{ '/docs/features/apply/' | relative_url }}) ·
[Ports]({{ '/docs/features/ports/' | relative_url }}) ·
[Configuration]({{ '/docs/configuration/' | relative_url }})
