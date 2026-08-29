---
layout: default
title: Audit Log
description: Who changed what, when — and which colour means the firewall actually moved.
---

# Audit Log

Every administrative change, newest first. The viewer shows the last 200; the file
on disk keeps everything.

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/log" ext="png"
     alt="The audit log page: a filterable table with timestamp, colour-coded action, rule type, detail and user columns." %}
  <figcaption>The filter matches the wording on screen as well as the identifier stored on disk.</figcaption>
</figure>

## Only 10 entries carry colour

Colour outside the accent family always means firewall state — green live, amber
unconfirmed, red rolled back. If a merely informational event were tinted too, a
coloured tag would stop meaning anything.

| | Action | Reads as | Meaning |
|---|---|---|---|
| 🟢 | `apply_accepted` | Rules applied | Confirmed and live |
| 🟠 | `apply_started` | Apply started | Live but unconfirmed — the window is open |
| 🔴 | `apply_rolledback` | Rules rolled back | The window closed unconfirmed; the previous rules are back |
| 🔴 | `apply_failed` | Apply failed | The rules could not be pushed to the kernel |
| 🔴 | `rollback_failed` | Rollback failed | The worst outcome there is: the new rules did not take **and** the old ones did not come back |
| 🟢 | `boot_enforced` | Rules restored at startup | The stored rules were back in the kernel before anything else started |
| 🔴 | `boot_enforce_failed` | Rules could not be restored at startup | The machine came up and is not filtering — nothing on this list is worse. Also written when the panic marker cannot be read at all, when panic mode was engaged from the console while the restore was still writing, and whenever a panic teardown itself failed and the machine may still be filtering behind a marker that says it is not. The detail says which |
| 🔴 | `panic_engaged` | Panic mode engaged | A human at the console took the firewall down on purpose. Deliberate does not make it neutral: the machine is unfiltered either way |
| 🟢 | `panic_resumed` | Panic mode ended | The console put the firewall back to filtering |
| 🔴 | `resume_restore_skipped` | Resume could not restore the rules | Resume cleared the panic marker but an apply held the slot, so the stored rules never made it back — the machine is left exactly as unfiltered as `boot_enforce_failed` describes |
| ⚪ | everything else | Rules saved, Options saved, Apply refused — panic mode is engaged, Rollback skipped — panic mode is engaged, … | Something was staged, or an attempt changed nothing live |

> **`rules_saved` is neutral, not green.** Saving stages a change and leaves the
> running firewall untouched. The same goes for `apply_refused_panic` and
> `rollback_skipped`. Neither leaves the firewall doing anything new. One is an
> apply that was refused, or whose rules were taken straight back down when
> panic mode appeared underneath it. The other is a rollback that left the
> kernel as the console's teardown had it. `rollback_skipped` does still revert
> the stored rules to the set that was live before the apply; what it skips is
> the write into a table
> nobody wants filled. The news in both cases is `panic_engaged`, which is red a
> few lines away. Only the 10 actions above describe what the firewall is
> actually doing, however consequential an event feels.

## The nine login events

New in 2.8, where there were none at all: this page used to send you to
`journalctl -u easywall-web` for a failed login.

| Action | Reads as | When |
|---|---|---|
| `login_ok` | Signed in | A completed sign-in, both steps if a factor is enrolled |
| `login_failed` | Sign-in failed | Wrong username or wrong password |
| `login_2fa_failed` | Second factor failed | Wrong code, or a recovery code that is not one of the eight |
| `login_recovery_used` | Recovery code used | The detail says how many are left |
| `login_ratelimited` | Sign-in attempts blocked | Five attempts inside ten minutes from one address |
| `logout` | Signed out | The button, not a timeout |
| `totp_enabled` · `totp_disabled` | Second factor switched on / off | From the password page |
| `recovery_codes_regenerated` | New recovery codes issued | The eight previous ones stopped working at that moment |

**None of them carries colour**, and that is the same rule the table above
states: colour means the firewall moved. A sign-in does not move it.

**Three of them are folded.** `login_failed`, `login_2fa_failed` and
`login_ratelimited` are the three a stranger can trigger. A burst from one
address becomes two lines — the first immediately, then a summary sixty seconds
later saying how many followed. Without that, forty addresses knocking for an
hour would push everything else off the 200 lines this page shows. Beyond 1024
distinct addresses inside one window the rest are counted together, so the volume
is still recorded while the memory is not something a stranger chooses.

**The username is never recorded**, on a failed sign-in or a successful one. It
would be foreign text in the record, and with exactly one account it says
nothing. The `user` column says `web` for all nine.

## The columns

| Column | Holds |
|---|---|
| Timestamp | Clock time today, day and month before that. The full value is in the title attribute |
| Action | The identifier, rendered in your language |
| Rule type | `tcp`, `udp`, `blacklist`, `whitelist`, `forwarding`, `custom`, or `all` |
| Detail | What changed — the addresses added and removed, or the settings that moved |
| User | The **process** that wrote the entry, not the person — one of four values, listed below |

## The address is the peer, and says when it is not

easywall records the TCP peer: whoever actually opened the connection — unless
that peer is on `trusted_proxies`, in which case it records what
`X-Forwarded-For` names instead. Without an entry there, easywall-web
terminates TLS itself and is not assumed to sit behind a trusted proxy. An
address in the firewall's own audit log that any client could choose would be
worse than no address at all.

Behind a reverse proxy the peer **is** the proxy, so every login shows the same
address. When that happens the entry says so: the detail reads the address
followed by a **via proxy** chip. The line in `/var/log/easywall/audit.log`
carries the token `via-proxy`, so `grep 'via-proxy' audit.log` finds every one
of them.

## The user column

It names the process, and since 2.7 there are four of them:

| Value | Written by |
|---|---|
| `web` | the web interface — every rule, option and setting change, and every apply you start from a page |
| `core` | the daemon itself, for work no operator asked for in the moment: the boot restore and the restore that follows the end of panic mode |
| `console` | `easywall-core panic` or `resume`, carried out by the running daemon on the console's behalf |
| `console-no-daemon` | the same two commands with no daemon running, where the console tool writes the marker and the entry itself — the lockout path, so it says which process was there |

## What is not in it

| Not recorded | Where to look instead |
|---|---|
| Which account made a change | nowhere. `web` names the process, not the person, because the socket protocol carries no identity yet — [roadmap]({{ '/docs/roadmap/' | relative_url }}) |
| Read-only page views | nowhere — not recorded at all |
| Edits made directly to `easywall.toml` | your own change management |

> **The detail column was empty until 2.5.0.** Every save wrote a blank, so the
> column meant to answer *what changed* was a dash on every line. It now names
> the addresses that came and went, counts the entries for rule kinds whose
> members are structures, and names the settings that moved. Long lists are
> capped at six names plus a count.

## On disk

One JSON object per line, append-only, never truncated by easywall:

```bash
tail -f /var/log/easywall/audit.log
```

```json
{"time":"2026-08-09T14:25:41Z","action":"rules_saved","rule_type":"blacklist","detail":"added 203.0.113.7, removed 192.0.2.1","user":"web"}
{"time":"2026-08-09T14:25:43Z","action":"options_saved","rule_type":"","detail":"changed port_scan, tcp_rst_flood","user":"web"}
{"time":"2026-08-09T14:26:02Z","action":"apply_accepted","rule_type":"all","detail":"","user":"web"}
{"time":"2026-08-10T06:14:07Z","action":"boot_enforced","rule_type":"all","detail":"daemon start","user":"core"}
{"time":"2026-08-10T09:02:55Z","action":"panic_engaged","rule_type":"all","detail":"the firewall was taken down from the console","user":"console"}
```

Each line stores its time as RFC 3339 in UTC. The interface converts it to the
server's local zone for display. It also keeps the stored value in the
`title` attribute of the cell, so hovering shows the exact instant the core
recorded.

`rollback_failed` is the one worth alerting on: it means the new rules did not
take **and** the previous ones did not come back.

Line-oriented JSON ships straight into Filebeat, Promtail or any log collector.
Rotation is `logrotate`'s job — the Debian package installs a config for it.

## When it looks wrong

| Symptom | Cause |
|---|---|
| The table is empty | Nothing recorded yet, or the core cannot write to `log_dir` |
| Older entries missing | The viewer caps at 200. The file has them all |
| A search finds nothing you know is in the log | The filter searches the same newest 200, not the file. `grep` the file for anything older |
| Wrong timezone | Timestamps are stored in UTC and shown in the server's local zone. If the interface disagrees with your clock, it is the *server's* zone that is off — `timedatectl set-timezone Europe/Berlin`, then restart `easywall-web`. In Docker, set `TZ` on the container |
| A filter finds nothing | It matches action, rule type, detail and user; not the timestamp — and only within the 200 entries the page was given |
