# Running the interface locally

How-to: you have a task, look at the interface, not read about it.

## Starting it

```bash
scripts/demo-server.sh
```

No `easywall-core`, no root, no nftables — the server runs against an
in-memory mock (`demo_mode = true`). State lives under
`$EASYWALL_DEMO_DIR` (default `~/.local/share/easywall-demo`), the address
under `$EASYWALL_DEMO_ADDR` (default `127.0.0.1:12227`). Signs in as
`admin` / `ui-check-password-2026` — the same account `scripts/ui-check.mjs`
uses, so a session started by hand and one driven by the script are
interchangeable.

## The certificate

`mkcert -install` trusts a local CA in Chrome once. For years the demo server
never used that CA — it generated its own self-signed certificate — so every
review session still opened on an interstitial. `scripts/demo-server.sh` asks
mkcert for a leaf signed by that CA and points `easywall-web` at it; Chrome
then trusts the demo with no click-through.

Two things that void the fix, both learned the hard way:

| Trap | Why it bites |
|---|---|
| A different port | A browser exception is per-*origin* — scheme, host **and** port. Changing `$EASYWALL_DEMO_ADDR` costs a fresh interstitial on the self-signed path. |
| A scratch directory under `/tmp` | Cleared on reboot, so the certificate was silently regenerated and any exception already granted stopped matching it. This is what cost a day before this script existed. |

No mkcert CA on the machine? The script falls back to `easywall-web`'s own
self-signed certificate and says so. Trusting a CA is a change to your trust
store, so `mkcert -install` is something you run yourself, once — the script
never runs it for you.

## Three traps

| Trap | What happens |
|---|---|
| Templates are parsed at startup | Editing a template needs a restart; a CSS rebuild alone will not show the change. |
| The login rate limiter | 5 attempts per 10 minutes per IP. A sweep that signs in once per viewport trips it and silently screenshots the login page instead. Restarting the server is the only reset — it is an in-memory package var. |
| The self-signed interstitial can't be automated | The Chrome extension cannot click through it. On the fallback path a human has to, once per browser per origin. |

## The documentation site

`docs/` is Jekyll, and there is no ruby on this machine — `scripts/docs-build.sh`
builds it in a container instead, producing `docs/_site` and its search index.

One trap in what comes out: `docs/_site/features/*`, `docs/_site/installation/*`
and the other top-level paths are redirect stubs to production, not the real
pages — the built pages live under `docs/_site/docs/`. Pointing a browser at a
top-level path silently reviews the live site and still looks correct.
