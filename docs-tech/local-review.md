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

## Driving it in a browser

```bash
scripts/demo-server.sh &        # or in another terminal
npm run check:ui
```

Nothing to set. The script finds the demo's `web.toml` under
`$EASYWALL_DEMO_DIR` (default `~/.local/share/easywall-demo`) and falls back to
`/etc/easywall/web.toml` for a run against a real installation; `EASYWALL_CONFIG`
overrides both. That default used to be `/etc/easywall/web.toml` alone — a file
`demo-server.sh` has never written — so a bare `npm run check:ui` reported
"could not read the password hash" and every run needed the variable set by
hand. It is the check that caught this release's worst defect, and friction on
it is what gets it skipped.

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

## The integration suite, without host root

`sudo go test -tags integration ./internal/core/...` runs against the host's own
kernel and its `inet easywall` table — every apply and flush in the suite happens
for real, on the same firewall the maintainer's machine is running. Rootless
podman with its own network namespace gives the tests a real kernel to apply
against without either: a throwaway netns is flushed when the container exits,
not the host's.

```bash
podman run --rm --cap-add=NET_ADMIN --cap-add=SYS_ADMIN --security-opt unmask=ALL \
  -v "$PWD:/src:Z" -w /src docker.io/library/golang:1.27 \
  sh -c 'apt-get update -qq && apt-get install -y -qq nftables iproute2 iputils-ping >/dev/null && \
         go test -tags integration ./internal/core/... -v'
```

The image tag has to be at least the `go` directive in `go.mod`, or the run
stops before a test with `go.mod requires go >= …` — the container sets
`GOTOOLCHAIN=local`, so it will not fetch a newer toolchain the way the host
does. This command said `golang:1.25` until 2.15 and by then did not run at all.

`--cap-add=NET_ADMIN` alone is not enough: `TestMain` re-execs into a fresh
network namespace via `CLONE_NEWNET`, which needs `CAP_SYS_ADMIN` too — without
it the child re-exec fails before a single test runs, surfacing only as a bare
`exit status 1`.

The other two flags matter for a different reason: without them the
router-based tests do not fail, they **skip** — "ip is not installed", "cannot
create a network namespace" — which reads exactly like a pass in a scrollback
nobody reads closely. `newRouter` shells out to `ip` and `ping`, so
`iproute2 iputils-ping` has to be on the install line beside `nftables`; and
`newRouter` also writes `/proc/sys/net/ipv4/ip_forward`, which podman's default
masking makes read-only inside the container regardless of the network
namespace owning it, so `--security-opt unmask=ALL` has to be on the command
line, not the install list. Both are scoped to the container's own throwaway
namespace and files; neither touches the host. Keep the host command beside
this one: a container kernel is not the host kernel, and either may need
running.

## The documentation site

`docs/` is Jekyll, and there is no ruby on this machine — `scripts/docs-build.sh`
builds it in a container instead, producing `docs/_site` and its search index.

One trap in what comes out: `docs/_site/features/*`, `docs/_site/installation/*`
and the other top-level paths are redirect stubs to production, not the real
pages — the built pages live under `docs/_site/docs/`. Pointing a browser at a
top-level path silently reviews the live site and still looks correct.
