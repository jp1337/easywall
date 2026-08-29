---
layout: default
title: Blacklist
description: A list of addresses that are dropped before anything else is considered.
---

# Blacklist

A list of addresses that are dropped. It is consulted before every other rule
that can accept a packet, which is the only thing about it you really need to
remember.

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/blacklist" ext="png"
     alt="The blacklist editor: a textarea of blocked addresses with a live entry count, per-line validation, and context cards explaining what gets blocked and that the order matters." %}
  <figcaption>Every line is validated as you type, and the line number is named when one does not parse.</figcaption>
</figure>

## The order

{% include themed-figure.html base="/assets/diagrams/rule-order" ext="svg"
   alt="Decision flow for an incoming packet: loopback first, then the IPv6 mode, which accepts or drops all IPv6 outright unless it is set to filter; then established connections and ICMP, then protection modules, then Docker bridge networks, then the blacklist which drops, then the whitelist which accepts every port, then open ports, then custom rules, and finally the chain policy which drops." %}

**The blacklist wins.** An address on both lists is dropped, because the blacklist is
evaluated first. A narrow allow inside a wide block does not work — take the entry off
the blacklist instead.

> **Fixed in 2.11.** The SSH brute-force chain is consulted before the blacklist
> and used to accept, so a blacklisted address could still open an SSH connection
> while it stayed under the rate limit. It now returns, and the blacklist decides.

The list consulted immediately after this one is the
[whitelist]({{ '/docs/features/whitelist/' | relative_url }}), which accepts.

## What it does

| | |
|---|---|
| Effect | DROP |
| Evaluated | before the whitelist, and before the port rules |
| Skips the protection modules | no — those run first |
| Use it for | a scanner, an abusive network |

## Accepted input

One entry per line. Lines starting with `#` are comments; blank lines are ignored.

| Form | Example |
|---|---|
| IPv4 address | `192.0.2.42` |
| IPv4 network | `198.51.100.0/24` |
| IPv6 address | `2001:db8::1` |
| IPv6 network | `2001:db8::/32` |

The counter under the editor counts real entries — comments and blanks do not inflate it.

## When it does not work

| Symptom | Cause |
|---|---|
| A blacklisted address still gets through | The connection was already established; the list only affects new ones |
| An address you also whitelisted is still blocked | The blacklist is checked first, and it wins. Take the entry off the blacklist |
| Nothing changed after saving | Saving stages. It goes live on [Apply]({{ '/docs/features/apply/' | relative_url }}) |
| The editor names a line number | That line is not a valid address or CIDR; the message says why |

Need the network a single address belongs to?

```bash
whois 198.51.100.42 | grep -i route
```

Logging blacklist hits is a switch on the [options page]({{ '/docs/features/filters/' | relative_url }});
they appear in the kernel log with an `easywall blacklist:` prefix.
