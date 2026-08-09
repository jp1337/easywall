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

## Only four entries carry colour

Colour outside the accent family always means firewall state — green live, amber
unconfirmed, red rolled back. If a merely informational event were tinted too, a
coloured tag would stop meaning anything.

| | Action | Reads as | Meaning |
|---|---|---|---|
| 🟢 | `apply_accepted` | Rules applied | Confirmed and live |
| 🟠 | `apply_started` | Apply started | Live but unconfirmed — the window is open |
| 🔴 | `apply_rolledback` | Rules rolled back | The window closed unconfirmed; the previous rules are back |
| 🔴 | `apply_failed` | Apply failed | The rules could not be pushed to the kernel |
| ⚪ | everything else | Rules saved, Options saved, … | Something was staged; nothing live moved |

> **`rules_saved` is neutral, not green.** Saving stages a change and leaves the
> running firewall untouched. Only the four `apply_*` actions describe what the
> firewall is actually doing, however consequential an edit feels.

## The columns

| Column | Holds |
|---|---|
| Timestamp | Clock time today, day and month before that. The full value is in the title attribute |
| Action | The identifier, rendered in your language |
| Rule type | `tcp`, `udp`, `blacklist`, `whitelist`, `forwarding`, `custom`, or `all` |
| Detail | Whatever the core recorded. Usually empty — see below |
| User | The account that made the change |

## What is not in it

| Not recorded | Where to look instead |
|---|---|
| Logins, failed logins, logouts | `journalctl -u easywall-web` |
| Read-only page views | nowhere — not recorded at all |
| Edits made directly to `easywall.toml` | your own change management |
| **What changed**, in most cases | the detail column is empty for every save and apply |

> **The detail column is nearly always a dash.** The core passes something only
> twice: `timeout` on a rollback, and the nftables error on a failure. Recording
> *what* a save changed would make the log far more useful and is a gap, not a
> design decision.

## On disk

One JSON object per line, append-only, never truncated by easywall:

```bash
tail -f /var/log/easywall/audit.log
```

```json
{"time":"2026-08-04T14:25:43Z","action":"apply_accepted","rule_type":"all","detail":"","user":"admin"}
```

Line-oriented JSON ships straight into Filebeat, Promtail or any log collector.
Rotation is `logrotate`'s job — the Debian package installs a config for it.

## When it looks wrong

| Symptom | Cause |
|---|---|
| The table is empty | Nothing recorded yet, or the core cannot write to `log_dir` |
| Older entries missing | The viewer caps at 200. The file has them all |
| Wrong timezone | Timestamps follow the server clock — `timedatectl set-timezone` |
| A filter finds nothing | It matches action, rule type, detail and user; not the timestamp |
