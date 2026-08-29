---
layout: default
title: Whitelist
description: Addresses that reach every port, whether it is open or not — and the way back into your own machine.
---

# Whitelist

A list of addresses that are accepted. Not accepted *for the ports you opened* —
accepted for **every port**, open or not. That is what makes it the way back
into a machine whose rules you are about to change, and it is what makes a wide
entry expensive.

> **A whitelisted source reaches services you never opened.** It does not pass
> the port rules, it skips them. Prefer a single address over a range, and a
> range over a whole network.

## Where it sits

The whitelist is consulted **after** the [blacklist]({{ '/docs/features/blacklist/' | relative_url }})
and **before** the port rules. An address on both lists is dropped. The full
packet order, for all of it at once, is the `rule-order` diagram on the
[blacklist page]({{ '/docs/features/blacklist/#the-order' | relative_url }}).

## Your way back in

Put the address you administer the host from on this list **before** you start
changing port rules.

| | |
|---|---|
| What it survives | a closed SSH port, and every port rule you change |
| What it does **not** survive | the protection modules — they run before the whitelist, so a packet a module drops never reaches it |
| Why that rarely bites | the rate limits are counted per source address, so somebody else's flood cannot spend your budget |
| The one exception | the [bogon filter]({{ '/docs/features/filters/' | relative_url }}) reads this list. It drops private source addresses, so without that it would drop you for administering the host from one — and the entry meant to prevent exactly that could never be reached |

Together with the [acceptance window]({{ '/docs/features/apply/' | relative_url }})
that is two independent ways not to lose access to your own machine.

## Accepted input

One entry per line. Lines starting with `#` are comments; blank lines are
ignored. The counter under the editor counts real entries — comments and blanks
do not inflate it.

| Form | Example |
|---|---|
| IPv4 address | `192.0.2.42` |
| IPv4 network | `198.51.100.0/24` |
| IPv6 address | `2001:db8::1` |
| IPv6 network | `2001:db8::/32` |

## When it does not work

| Symptom | Cause |
|---|---|
| A whitelisted address is still blocked | It is on the blacklist too — that is checked first. Or a protection module dropped it, which happens before the whitelist. The bogon filter is the exception: it honours this list |
| Nothing changed after saving | Saving stages. It goes live on [Apply]({{ '/docs/features/apply/' | relative_url }}) |
| The editor names a line number | That line is not a valid address or CIDR; the message says why |
| You allowed too broad a range | Remove it, save, apply. If it already locked you out, do nothing — the window rolls it back |
