---
layout: default
title: Notifications
description: Send a message to a webhook or an ntfy topic when an apply undid itself, when one was confirmed, when panic mode changed, or when sign-ins keep failing.
---

# Notifications

easywall can tell you that something happened instead of waiting for you to open the
dashboard. **Notifications** in the sidebar holds the whole setting: one destination,
four triggers, one Save button.

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/notify" ext="png"
     alt="The notifications page: a Send to selector, an address field, four trigger checkboxes, and Save notifications beside a Send a test button." %}
  <figcaption>Nothing leaves this host until an address is set and a trigger is ticked.</figcaption>
</figure>

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

| Trigger | Sent when | Carries |
|---|---|---|
| **An apply undid itself** | the window closed unconfirmed, or you ended it by hand | which of the two it was |
| **An apply was confirmed** | you confirmed inside the window and the rules are live | the event, nothing more |
| **Panic mode started or ended** | on either edge, including `easywall-core panic` at the console | which edge, in words |
| **Repeated failed sign-ins** | five failures from one address inside five minutes | the count and the address |

Nothing is sent for a trigger you leave unticked. The four take effect when you press
**Save notifications**, not while you are ticking them.

### The three the firewall raises

easywall reads its own status every 15 seconds. That is one socket round trip to
the daemon on this host, and nothing to the network. The tick is longer than the
status cache holds, so each one is a real ask rather than a free read. Confirmed
and rolled back are states that stay, not instants that pass. A read landing after
the 120-second window closed still finds the outcome, so nothing races that window.

A rollback carries the reason it happened, and the two reasons are not flattened
into one: a window that expired is not a window an operator ended.

### The one about sign-ins

Five is not a number chosen here: it is the rate limiter's own ceiling, so the
message is worth the same as easywall refusing that address. The same count, not
the same moment: the limiter is a token bucket that refills, and this is a fixed
five-minute window. Five failures spread over four minutes notify while the limiter
still has tokens. The address then stays quiet for fifteen minutes, because the
second hour of one attack is not news. Every other address keeps its own count.

## Prove it arrives

**Send a test** posts one notification now, to the address in the field, whatever
the four switches say. A test that stayed silent because a trigger is off would
prove nothing about the endpoint. Under the triggers the page then shows when
easywall last sent something, or what went wrong the last time it tried.

The public demo sends nothing, and says so before you press the button.

## What arrives

### A webhook

`POST`, with a JSON body:

```json
{
  "event": "rolled_back",
  "severity": "warning",
  "detail": "timeout",
  "host": "gateway",
  "version": "2.20.0",
  "time": "2026-09-15T14:25:43Z"
}
```

`severity` is one of `info`, `warning` or `critical`, and `time` is UTC. `event` is
one of **five** values:

| `event` | Raised by |
|---|---|
| `test` | **Send a test** — the first one your receiver will ever see, and the one it gets with every switch off |
| `rolled_back` · `accepted` | the acceptance window, either way it ended |
| `panic` | panic mode, on either edge |
| `failed_logins` | the sign-in threshold |

A receiver written as a four-way switch over the triggers drops `test`, which is
the message the button exists to produce.

### ntfy

`POST`, `text/plain`. The body is the `detail` above; the rest travels in headers:

| Header | Value |
|---|---|
| `Title` | this host's name, then the event |
| `Priority` | `3` for info, `4` for warning, `5` for critical |
| `Tags` | the event name |

ntfy has five priorities and easywall uses three. Nothing it has to say is worth
ntfy's lowest, and the highest is kept for the event that means this host is not
filtering.

## What is not promised

There is no queue and no delivery guarantee. A delivery that fails is tried once
more and then dropped, with a line in the journal saying so. If the host has no
route out when the rollback happens, that notification is gone.

The audit log is the record; a notification is a convenience. Reading it as a
second record is the one way this feature can mislead you. So the page says what
it last managed to send, rather than implying every attempt landed.

> **Redirects are refused.** A destination that answers with one is never
> followed. Otherwise whoever controls its DNS could point your notifications at
> somebody else.

Of the four requests easywall makes, this is the only one whose destination it
does not name. The other three are listed under
[Security]({{ '/docs/security/' | relative_url }}#every-request-that-goes-out).
