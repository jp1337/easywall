---
layout: default
title: Notifications
description: Send a message to a webhook or an ntfy topic when an apply undid itself, when one was confirmed, when panic mode changed, or when sign-ins keep failing.
---

# Notifications

easywall can tell you that something happened instead of waiting for you to open the
dashboard. **Notifications** in the sidebar holds the whole setting: one destination,
four triggers, one Save button.

## Set a destination

| Send to | Address |
|---|---|
| **Nothing** | Notifications are off. This is the default |
| **Webhook** | Any HTTP endpoint that accepts a POST with a JSON body |
| **ntfy** | A topic URL, for example `https://ntfy.sh/your-private-topic` |

Only `http://` and `https://` are accepted. Nothing else is a web endpoint, and the
address is refused before it reaches the configuration file.

> **The address is a credential.** Anyone who has it can post to your topic, and on a
> public ntfy server anyone who guesses it can read yours. Pick a topic name nobody
> will guess, or run your own.

## Choose what you hear about

| Trigger | Sent when |
|---|---|
| **An apply undid itself** | The acceptance window closed with no confirmation and the rules rolled back |
| **An apply was confirmed** | You confirmed inside the window and the rules are live |
| **Panic mode started or ended** | The firewall was opened, or closed again |
| **Repeated failed sign-ins** | Sign-ins to the web interface keep failing |

Nothing is sent for a trigger you leave unticked. The four take effect when you press
**Save notifications**, not while you are ticking them.

Under the triggers, the page shows when easywall last sent something — or what went
wrong the last time it tried.
