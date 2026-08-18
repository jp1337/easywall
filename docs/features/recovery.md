---
layout: default
title: Recovery & Panic Mode
description: The rules come back after a restart, and the console tool for when they should not have.
---

# Recovery & Panic Mode

Two things, and they are opposites. After a restart, the rules you last confirmed
are back in force — no window, no delay. And when that is exactly what has shut
you out, `easywall-core` has a way back that does not need the web interface.

## After a restart, the rules are back

Before 2.7, nftables forgot everything on reboot. The table came up empty, and
the machine stayed unfiltered until somebody opened the web interface and
pressed Apply — which meant a reboot was also the accidental escape route: lock
yourself out, restart the machine, and you are back in. That door is closed now.
`easywall-core` puts the stored **Current** rule set into the kernel before the
socket accepts a single connection.

**There is no acceptance window on this**, and that looks like a breach of
easywall's central promise until you look at what `Current` already is: a rule
set that has already survived one. It got there by being applied and then
confirmed, or by surviving a rollback nobody had to trigger. Restoring it is not
a new change taking effect unconfirmed — refusing to restore it would be the
actual change, and the one nobody asked for.

A window here could not work even if you wanted it to. Nobody is present at boot
to confirm, so it would expire; the rollback would install `Backup`, which
nobody confirmed either; and there is no second window behind that one to catch
it. A loop with no exit — so there is not a loop.

## What to do when you have locked yourself out

In order, because the first one costs nothing and undoes itself:

1. **Wait.** If what locked you out was an apply — not a reboot — the acceptance
   window is still running. Do nothing for up to 120 seconds (or whatever
   [the window is configured to]({{ '/features/system-settings/' | relative_url }})),
   and the previous rules come back on their own. This is the same recovery
   [Applying Rules]({{ '/features/apply/' | relative_url }}) describes — it costs
   nothing to try, and it needs nothing but time.
2. **Only then, the console.** If the window has already closed, if the acceptance
   window was switched off, or if what locked you out was the restart itself —
   reach for a terminal on the machine.

```bash
easywall-core status    # is the firewall enforcing, and since when?
easywall-core panic     # take the firewall down, right now
easywall-core resume    # end panic mode and put the stored rules back
```

### `status`

```
$ easywall-core status
firewall:   enforcing
acceptance: accepted
last apply: 2026-08-16T09:12:03Z
```

When the daemon cannot be reached at all — crashed, or not started yet — `status`
falls back to reading the panic marker directly rather than asking a socket
nothing answers on:

```
$ easywall-core status
daemon:     not running
panic mode: engaged — the rules will NOT come back on start
            run `easywall-core resume` first
```

| Exit code | Meaning |
|---|---|
| `0` | The firewall is enforcing, **or** panic mode is engaged and says it was meant not to be — either way, the daemon answered |
| `2` | The firewall is not filtering when it should be, **or** the daemon is not running at all — whether or not panic mode is engaged |

That disjunction on `2` is deliberate, not an oversight: a machine with no
daemon running is never in the state it should be, because nothing will put the
rules back once the daemon starts and panic mode ends — so a monitoring check
sees `2` either way, and only the printed message tells you which case you are
in.

### `panic`

```
$ easywall-core panic
The firewall is down. This machine is unfiltered, and stays that way
across a restart until you run `easywall-core resume`.
```

`panic` flushes `table inet easywall`. Every rule, every protection module, every
port you opened — gone, immediately, and the machine is reachable on all of them.
This is the way back into a machine your own rules have shut you out of, at the
cost of the thing that shuts anyone else out too. Run it only at the console of a
machine you already have a more physical way to reach.

`panic` waits for the same lock an in-flight apply holds while it writes custom
rules through `nft`. If it appears to hang, that is almost certainly the wait —
it is not stuck.

### `resume`

```
$ easywall-core resume
Panic mode is over and the stored rules are back in force.
```

`resume` clears the marker and restores `Current` — the same restore a restart
performs, run on demand. If the daemon is not running, `resume` can only clear
the marker; it says so, and names the command to start the service so the
restore actually happens.

If the daemon *is* running but an apply is already in flight, `resume` still
clears the marker — panic mode has genuinely ended — but the restore itself is
refused and the command exits `1`. The rules are not back yet in that case.
Run `status` to see when the apply finishes, then `resume` again.

## Panic mode survives a restart, deliberately

The whole reason panic mode exists is to give you back the escape hatch 2.7
takes away. If a reboot undid it, the next restart would put you right back
behind the rules you just tore down for exactly that reason — the fix for one
lockout becoming the next one. So while panic mode is engaged, the startup
restore does not run, and it stays that way until you run `resume`.

While the marker exists, two more things change: an apply is refused rather than
queued behind a table that is not there, and the acceptance rollback stops short
of the kernel. It still reverts the stored rules, so `Current` never keeps a set
nobody confirmed — that matters, because the next restore installs `Current` with
no acceptance window of its own — but it writes nothing into the table the console
just took down. Both are covered on
[Security]({{ '/security/' | relative_url }}).

Engaging panic mode also wins a race it used to lose. The daemon re-reads the
marker after every write to the table, not only before one, so a `panic` that
lands while an apply or a startup restore is mid-write has the rules taken down
again rather than left live behind a marker that says the machine is unfiltered.

## The marker file

Panic mode is recorded in one file:

```
/var/lib/easywall/panic
```

Its presence *is* the state — there is nothing inside it to read. Anyone looking
at a directory listing of `/var/lib/easywall` and finding a file named `panic`
sitting beside `rules.json` should be able to guess what it means without
opening it, and that is the point of naming it plainly rather than something
that has to be looked up. It is owned by root, mode `0600`: the web process
learns about panic mode over the socket, in the status reply, and never reads
this path itself.

## Why this ends at the console, not in the browser

The interface shows panic mode as a banner across every page — and the banner
has no button. That omission is deliberate, not missing work: a control there
would let the network-facing process re-arm a firewall a human just disarmed at
the machine, on purpose, using a stolen session or nothing more than a browser
tab left open. `easywall-web` has no path to the kernel by design, and panic
mode is the one state where staying that way matters most — the moment somebody
chose to make the machine reachable at any cost, the last thing the design
should do is hand that decision back to the process reachable from the network.
Ending panic mode takes the console, on purpose, every time.

---

**Next:** [Security]({{ '/security/' | relative_url }}) ·
[Audit Log]({{ '/features/audit-log/' | relative_url }}) ·
[Applying Rules]({{ '/features/apply/' | relative_url }})
