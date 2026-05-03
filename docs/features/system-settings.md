---
layout: default
title: System Settings
description: Configure the easywall acceptance window — the two-step safety mechanism that auto-rolls back firewall changes if you lose connectivity.
---

# System Settings

The System Settings page (`GET /POST /system`) controls the **acceptance window**: the safety mechanism that automatically restores your previous firewall rules if a newly applied rule set is not explicitly confirmed within a configurable time limit.

Changes saved here are written directly to the `[acceptance]` section of `easywall.toml` via IPC to the core process and take effect immediately. No restart is required.

## The Acceptance Window

When you apply staged rules, the core switches the running firewall to the new rule set and starts a countdown timer. You must return to the web UI and confirm within the acceptance window. If the timer expires without confirmation, the core automatically rolls back to the rules that were active before you clicked Apply.

This prevents you from locking yourself out: if a new rule set blocks your SSH or HTTPS port, the rollback fires and your access is restored.

```
Apply staged rules
       │
       ▼
New rules active  ──── acceptance timer starts ────►  Timer expires
       │                                                     │
       ▼                                                     ▼
[you confirm]                                       Auto-rollback fires
Rules stay active                                   Previous rules restored
```

## Settings

### Enabled

Toggles the acceptance window on or off for future applies.

- **On (recommended):** Every apply starts the timer. You must confirm before the duration elapses.
- **Off:** Applied rules become permanent immediately. There is no automatic rollback.

<div class="callout callout-warning">
  <svg class="callout-icon" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd"/></svg>
  <div class="callout-content">
    <strong>Warning</strong>
    <p>Disabling the acceptance window removes the automatic rollback safety net. If you apply a rule set that blocks your own access, there is no recovery path short of direct console access to the server.</p>
  </div>
</div>

### Duration

How many seconds the core waits for confirmation before rolling back. Accepted range: **10–3600 seconds**.

| Duration | Behaviour |
|---|---|
| 10–30 s | Very short window — leaves almost no time to confirm. Avoid unless you have a fast, reliable connection and the web UI is immediately reachable after applying. |
| 60–300 s | Recommended range. Enough time to confirm from a browser tab, even on a slow connection, without leaving the server exposed for long if something goes wrong. |
| 300–3600 s | Long window — useful when applying rules from an automated pipeline where confirmation may be delayed, or during maintenance with a known slow confirmation path. |

<div class="callout callout-info">
  <svg class="callout-icon" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a.75.75 0 000 1.5h.253a.25.25 0 01.244.304l-.459 2.066A1.75 1.75 0 0010.747 15H11a.75.75 0 000-1.5h-.253a.25.25 0 01-.244-.304l.459-2.066A1.75 1.75 0 009.253 9H9z" clip-rule="evenodd"/></svg>
  <div class="callout-content">
    <strong>Recommendation</strong>
    <p>Start with <strong>120 seconds</strong>. It is long enough to confirm comfortably while still rolling back quickly if you lose access. Reduce it only once you are confident in your workflow.</p>
  </div>
</div>

## Saving Changes

1. Navigate to **System Settings** in the sidebar
2. Toggle **Enabled** or change the **Duration** field

Changes are saved **automatically** as soon as you toggle the switch or change the duration. A small confirmation toast appears in the bottom-right corner ("System settings saved" / "Save failed") and the new values are sent to the core over IPC and written to `easywall.toml`. The next apply operation will use the updated settings. There is no need to restart either the core or the web process — and no need to click the explicit Save button (it remains as a fallback for when JavaScript is disabled).

The same auto-save UX applies on the **Options** and **Network** pages: every toggle and numeric input persists on change, with the toast indicating success or failure.

## Configuration File Reference

The acceptance window maps to the `[acceptance]` section in `easywall.toml`:

```toml
[acceptance]
enabled  = true
duration = 120   # seconds
```

You can edit this file directly on the server if the web UI is unavailable, then send `SIGHUP` to the core process to reload the configuration without a full restart.

## Troubleshooting

**I saved new settings but the apply still uses the old duration**

Settings take effect for the next apply. If an apply is already in progress (timer is running), the in-flight timer is not affected — it will complete with the duration that was set when Apply was clicked.

**The acceptance window expired but I could not confirm in time**

Increase the duration to give yourself more time, or check what slowed down the confirmation (network latency, browser redirect, CSRF token expiry). The rollback is recorded in the [Audit Log]({{ '/features/audit-log/' | relative_url }}) with a `ROLLBACK` action.

**The Duration field rejects my value**

The valid range is 10 to 3600 (inclusive). Values outside this range are rejected to prevent both immediate lockout (too short) and excessively long exposure windows (too long).
