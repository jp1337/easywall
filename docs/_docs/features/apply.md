---
layout: default
title: Applying Rules
description: Nothing you edit reaches the firewall until you apply it — and an apply undoes itself unless you confirm.
---

# Applying Rules

Every other page **stages**. This one is where staged rules become the running
firewall, and it is the only page that changes what the kernel is doing.

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/apply" ext="png"
     alt="The Apply Rules page: a status card reading Idle with an Apply now button, beside cards explaining the three steps of applying and how long the confirmation window is." %}
  <figcaption>The status card answers one question — is the running firewall what you staged?</figcaption>
</figure>

## The three steps

{% include themed-figure.html base="/assets/diagrams/apply-flow" ext="svg"
   alt="State machine: editing leads to Staged, applying leads to Live, confirming within the window leads to Confirmed, and letting the window expire leads to Rolled back, from where the staged edits are still available." %}

| | | |
|---|---|---|
| 1 | **Rules go live** | The staged set is pushed to the kernel and starts filtering immediately |
| 2 | **You check** | Open a *second* connection — keep the current one — and confirm SSH and your services still answer |
| 3 | **Confirm, or do nothing** | Confirming keeps the new rules. Doing nothing restores the previous set when the window closes |

**Doing nothing is the recovery.** If the new rules cut your connection you cannot
click Confirm, and not confirming is exactly what brings the old rules back. The
window is 120 seconds by default, [configurable]({{ '/docs/features/system-settings/' | relative_url }})
from 10 to 3600.

## What the status card says

| State | Meaning | What to do |
|---|---|---|
| **Idle** | the running firewall matches what is staged | nothing |
| **Staged changes** | you have edits the firewall does not have yet | apply them |
| **Waiting for confirmation** | live but unconfirmed, the window is open | check a second connection, then confirm |
| **Confirmed** | the rules stay | nothing |
| **Rolled back** | the window closed unconfirmed; the previous rules are back | your staged edits are still there — review and apply again |

A rollback loses nothing you staged. It undoes what went **live**, and the edits
that caused it are still on their pages.

## While the window is open

- The countdown lives in the core daemon, not in your browser. Closing the tab,
  losing the connection or restarting `easywall-web` confirms nothing.
- Stopping `easywall-core` counts as not confirming: the rules roll back before it
  exits. A restart inside those two minutes is an ordinary event, and it used to
  leave the unconfirmed set in place with nothing left to undo it.
- A second apply is refused while one is running, with a message rather than a
  silent second timer.
- The window keeps the length it started with. Changing the duration mid-window
  affects the next apply.

## When the acceptance window is switched off

An apply is then final and there is no automatic way back. That setting exists for
machines you can physically reach — see
[system settings]({{ '/docs/features/system-settings/' | relative_url }}).

## When it does not work

| Symptom | Cause | Check |
|---|---|---|
| "Nothing staged to apply" | you saved nothing since the last apply | the [dashboard]({{ '/docs/features/dashboard/' | relative_url }}) says whether changes are pending |
| Applied, then everything came back as it was | the window closed without a confirmation | the [audit log]({{ '/docs/features/audit-log/' | relative_url }}) shows `apply_rolledback` with detail `timeout` |
| The apply failed outright | the kernel refused a rule — usually a [custom rule]({{ '/docs/features/custom-rules/' | relative_url }}) | `journalctl -u easywall-core` carries nft's own message |
| `rollback_failed` in the log | the new rules did not take **and** the old ones did not come back | the worst outcome there is; check the daemon's log and the running table with `nft list table inet easywall` |
| Your SSH dropped right after applying | that is the design | do nothing; the window restores the previous rules |
| An apply is "already running" | one window is open | confirm it, or wait for it to expire |

---

**Next:** [Dashboard]({{ '/docs/features/dashboard/' | relative_url }}) ·
[System settings]({{ '/docs/features/system-settings/' | relative_url }}) ·
[Architecture]({{ '/docs/architecture/' | relative_url }})
