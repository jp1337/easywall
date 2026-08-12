---
layout: default
title: Dashboard
description: What this firewall is enforcing right now — asked of the kernel, not assumed.
---

# Dashboard

The landing page after signing in. It answers one question before any other:
**is the firewall doing what you think it is?**

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/dashboard" ext="png"
     alt="The easywall dashboard: firewall status with acceptance state, pending changes and last apply; tiles counting TCP ports, UDP ports, blacklist, whitelist, custom rules and forwarding; and a recent-activity list." %}
  <figcaption>Counts come from the rule set the kernel is loaded with, not from what is staged.</figcaption>
</figure>

## The status card

| Reads | Means |
|---|---|
| **Active** | the core daemon is running and its table is loaded in the kernel |
| **Pending changes** | something is staged that the running firewall does not have — go to [Apply]({{ '/features/apply/' | relative_url }}) |
| **Acceptance** | whether the [window]({{ '/features/system-settings/' | relative_url }}) is on, and whether one is open right now |
| **Last applied** | when the running set was last pushed |

"Active" is **asked of the kernel** rather than inferred from the daemon being up.
Until 2.5.0 it reported the daemon's own opinion, so a table that had been flushed
by something else still read as live.

## The tiles

Six counts — TCP ports, UDP ports, blacklist, whitelist, custom rules, port
forwarding — each linking to its page. They describe the **loaded** rule set. If
you have staged an edit, the tile still shows what is enforced; that is the point
of the number.

## Recent activity

The last few entries from the [audit log]({{ '/features/audit-log/' | relative_url }}),
newest first, with a link to the full page. Empty on a fresh installation.

## Export and import

Top right. Export downloads the **staged** rule set as JSON; import replaces it.
Neither touches the running firewall — see
[Export & Import]({{ '/features/export-import/' | relative_url }}).

## The update notice

A banner appears when a newer release exists. The answer comes from a cache on
disk, refreshed in the background once a day, so it never delays the page; on a
host with no route out the failure is remembered for an hour rather than retried
on every load. `update_check = false` removes it entirely — see
[Configuration]({{ '/configuration/' | relative_url }}#the-update-check).

## When it looks wrong

| Symptom | Cause | Check |
|---|---|---|
| "Core daemon unreachable" | `easywall-core` is not running, or the socket is not reachable by the web user | `systemctl status easywall-core`, then `ls -l /run/easywall/core.sock` — it must be `root:easywall` |
| Status inactive, rules obviously working | another table is filtering; easywall only reports its own | `sudo nft list tables` |
| A count does not match what you edited | the tile shows the **loaded** set, and your edit is staged | apply it |
| Pending changes you did not make | a save from another browser, or an import | the [audit log]({{ '/features/audit-log/' | relative_url }}) names it |
| The activity list is empty | nothing recorded yet, or the core cannot write `log_dir` | `journalctl -u easywall-core` |

---

**Next:** [Applying rules]({{ '/features/apply/' | relative_url }}) ·
[Audit log]({{ '/features/audit-log/' | relative_url }}) ·
[Export & import]({{ '/features/export-import/' | relative_url }})
