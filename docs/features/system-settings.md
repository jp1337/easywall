---
layout: default
title: System Settings
description: The acceptance window — the one setting that decides whether a bad rule is recoverable.
---

# System Settings

One page, one idea: how long easywall waits for you to confirm before undoing what
you just applied.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="{{ '/assets/diagrams/apply-flow-dark.svg' | relative_url }}">
  <img src="{{ '/assets/diagrams/apply-flow-light.svg' | relative_url }}" alt="State machine: editing leads to Staged, applying leads to Live, confirming within the window leads to Confirmed, and letting the window expire leads to Rolled back, from where the staged edits are still available.">
</picture>

## Enabled

| | |
|---|---|
| **On** — the default | Every apply starts the timer. Not confirming restores the previous rules |
| **Off** | An apply is final. There is no automatic way back |

> **Do not switch this off on a remote host.** A rule that closes your own SSH port
> leaves console access as the only recovery. The setting exists for machines you can
> physically reach.

## Duration

10 to 3600 seconds. The field refuses anything outside that — too short is a lockout,
too long is an exposure window.

| | Suits |
|---|---|
| 10–30 s | Almost nothing. You will not confirm in time on a slow link |
| **60–300 s** | **Most cases. Start at 120** |
| 300–3600 s | Automated pipelines, or a maintenance window with a known slow path |

Long enough to open a second connection and test what you changed; short enough that
a lockout resolves itself before it costs you the afternoon.

## Saving

Changes persist the moment you toggle or type — a toast confirms it, and the value
goes to the core and into `easywall.toml`. No restart. The Save button stays for
browsers with JavaScript disabled. The [options]({{ '/features/filters/' | relative_url }})
and network pages behave the same way.

Editing the file directly works too; send `SIGHUP` to the core to reload:

```toml
[acceptance]
enabled  = true
duration = 120   # seconds
```

## When it does not behave

| Symptom | Cause |
|---|---|
| The apply used the old duration | A running timer keeps the value it started with. The next apply picks up the change |
| The window expired before you could confirm | Raise the duration. The rollback is in the [audit log]({{ '/features/audit-log/' | relative_url }}) as `apply_rolledback` with detail `timeout` |
| The field rejects a value | Outside 10–3600 |
