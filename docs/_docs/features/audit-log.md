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

## Only five entries carry colour

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
| ⚪ | everything else | Rules saved, Options saved, … | Something was staged; nothing live moved |

> **`rules_saved` is neutral, not green.** Saving stages a change and leaves the
> running firewall untouched. Only the five actions above describe what the
> firewall is actually doing, however consequential an edit feels.

## The columns

| Column | Holds |
|---|---|
| Timestamp | Clock time today, day and month before that. The full value is in the title attribute |
| Action | The identifier, rendered in your language |
| Rule type | `tcp`, `udp`, `blacklist`, `whitelist`, `forwarding`, `custom`, or `all` |
| Detail | What changed — the addresses added and removed, or the settings that moved |
| User | Always `web`. It names the **process**, not the person — see below |

## What is not in it

| Not recorded | Where to look instead |
|---|---|
| Which account made a change | nowhere. Every entry says `web`, because the socket protocol carries no identity yet — [roadmap]({{ '/docs/roadmap/' | relative_url }}) |
| Logins and failed logins | `journalctl -u easywall-web` |
| Logouts | nowhere, though a logout does end the session immediately |
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
```

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
| Wrong timezone | Timestamps follow the server clock — `timedatectl set-timezone` |
| A filter finds nothing | It matches action, rule type, detail and user; not the timestamp — and only within the 200 entries the page was given |
