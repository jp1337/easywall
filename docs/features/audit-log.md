---
layout: default
title: Audit Log
description: View a timestamped record of every firewall change made through easywall — who did what, and when.
---

# Audit Log

The Audit Log page (`GET /log`) shows a read-only table of the 200 most-recent events recorded by the easywall core. Every add, delete, apply, rollback, import, and export operation is written to `audit.log` — the viewer brings that log into the web UI without requiring shell access.

## Reading the Log

Navigate to **Audit Log** in the sidebar. The table displays entries newest-first, with one row per event:

| Column | Description |
|---|---|
| Timestamp | Date and time the action was recorded by the core (server local time) |
| Action | The operation performed — see colour coding below |
| Rule Type | Which rule category was affected: `tcp`, `udp`, `blacklist`, `whitelist`, `forwarding`, `custom`, or `—` for bulk operations |
| Detail | The specific value involved — a port number, an IP address, a CIDR, or a short description |
| User | The web UI username that triggered the action |

The table itself is static — there is no live-refresh, so reload the page to see events that occurred after you opened it. Within the loaded page, however, you can narrow down quickly with the search box (see below).

## Filtering Entries

Above the table is a search input labelled *Filter by action, detail, user…*. Typing in this box filters the rows live: with each keystroke (debounced at ~300 ms), the page sends the query to `GET /log/filter?q=…` and replaces only the table body with the matching rows. The match is case-insensitive substring across the **Action**, **Rule Type**, **Detail**, and **User** columns.

Examples:

- `alice` — show only events recorded for the user `alice`
- `forwarding` — show only forwarding rule changes
- `192.168` — show entries that mention the `192.168` IP range in the detail column

Clear the input to see all entries again. The filter operates on the 200 entries returned by `/log` — to search older history, read `audit.log` on disk directly.

## Action Colour Coding

Only actions that changed the state of the firewall carry colour. Everything else —
saving settings, importing or exporting a rule set, changing options — is shown as a
neutral tag.

That restriction is deliberate. In easywall, colour outside the blue accent family
always means firewall state: green is live, amber is unconfirmed, red is rolled back or
failing. If a merely informational event were tinted too, a coloured tag in the log
would become ambiguous — is this a state, or just emphasis? Scanning the log for what
actually happened to your firewall would get slower, not faster.

| Colour | Action | Meaning |
|---|---|---|
| Green | `rules_saved`, `rules_applied` | Rules were staged or pushed to the running firewall |
| Red | `rules_rolled_back` | The acceptance window expired or was triggered manually — the previous rules were restored |
| Neutral | everything else | Settings saved, options changed, rule set imported or exported |

See `DESIGN.md` in the repository root for the full rule.

## Limits and Retention

The viewer shows at most **200 entries**. Older entries remain in `audit.log` on disk but are not displayed. To access the full history, read the file directly on the server:

```bash
cat /var/log/easywall/audit.log
```

The log file is append-only and is never truncated by easywall. Rotate it with `logrotate` if disk space is a concern.

## What Is Not Logged

The audit log covers rule changes only. The following are not recorded:

- Login and logout events
- Failed authentication attempts
- Configuration changes to `easywall.toml` (see [System Settings]({{ '/features/system-settings/' | relative_url }}))
- Read-only page views

<div class="callout callout-info">
  <svg class="callout-icon" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a.75.75 0 000 1.5h.253a.25.25 0 01.244.304l-.459 2.066A1.75 1.75 0 0010.747 15H11a.75.75 0 000-1.5h-.253a.25.25 0 01-.244-.304l.459-2.066A1.75 1.75 0 009.253 9H9z" clip-rule="evenodd"/></svg>
  <div class="callout-content">
    <strong>Tip</strong>
    <p>Pair the audit log with your SIEM or alerting stack by tailing <code>audit.log</code> on disk. The format is one JSON object per line, suitable for ingestion by Filebeat, Promtail, or any line-oriented log shipper.</p>
  </div>
</div>

## Troubleshooting

**The table is empty**

The core has not recorded any events yet, or `audit.log` does not exist at the configured path. Check that the core process has write permission to its log directory and has been used at least once.

**Entries are missing from the table**

The viewer caps display at 200 entries. If you need older entries, read `audit.log` directly. The file is never modified by the viewer — all entries are preserved on disk.

**The timestamp is in the wrong timezone**

Timestamps reflect the server's local time as set by the operating system. Adjust the system timezone with `timedatectl set-timezone` if needed.
