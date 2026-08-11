---
layout: default
title: Blacklist & Whitelist
description: Two address lists. One drops before anything else is considered, the other accepts every port whether it is open or not.
---

# Blacklist & Whitelist

Two lists of addresses. One drops, one accepts — and the order between them is the
only thing you really need to remember.

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/blacklist" ext="png"
     alt="The blacklist editor: a textarea of blocked addresses with a live entry count, per-line validation, and context cards explaining what gets blocked and that the order matters." %}
  <figcaption>Both editors validate every line as you type, and name the line number when one does not parse.</figcaption>
</figure>

## The order

{% include themed-figure.html base="/assets/diagrams/rule-order" ext="svg"
   alt="Decision flow for an incoming packet: loopback first, then the IPv6 mode, which accepts or drops all IPv6 outright unless it is set to filter; then established connections and ICMP, then protection modules, then Docker bridge networks, then the blacklist which drops, then the whitelist which accepts every port, then open ports, then custom rules, and finally the chain policy which drops." %}

**The blacklist wins.** An address on both lists is dropped, because the blacklist is
evaluated first. A narrow allow inside a wide block does not work — take the entry off
the blacklist instead.

## What each list does

| | Blacklist | Whitelist |
|---|---|---|
| Effect | DROP | ACCEPT |
| Evaluated | before the whitelist | after the blacklist, before the ports |
| Reaches closed ports | — | **yes, every port** |
| Skips the protection modules | no — those run first | no — those run first, except the bogon filter, which reads this list |
| Use it for | a scanner, an abusive network | the address you administer from |

> **A whitelisted source reaches services you never opened.** It does not pass the
> port rules, it skips them. Prefer a single address over a range, and a range over a
> whole network.

## Accepted input

One entry per line. Lines starting with `#` are comments; blank lines are ignored.

| Form | Example |
|---|---|
| IPv4 address | `192.0.2.42` |
| IPv4 network | `198.51.100.0/24` |
| IPv6 address | `2001:db8::1` |
| IPv6 network | `2001:db8::/32` |

The counter under the editor counts real entries — comments and blanks do not inflate it.

## Your way back in

Put the address you administer the host from on the whitelist **before** you start
changing port rules. It survives a closed SSH port and every port rule you change.

**It does not exempt you from the protection modules.** Those are evaluated before the
whitelist, as the diagram above shows — a packet a module drops never reaches it. That
matters least for the rate limits, which are counted per source address, so somebody
else's flood cannot consume your budget.

The one exception is the [bogon filter]({{ '/features/filters/' | relative_url }}),
which reads the whitelist itself. It drops private source addresses, so without that
it would drop you outright for administering the host from one — and the whitelist
entry meant to prevent exactly that could never be reached.

Together with the [acceptance window]({{ '/architecture/' | relative_url }}) that is two
independent ways not to lose access to your own machine.

## When it does not work

| Symptom | Cause |
|---|---|
| A whitelisted address is still blocked | It is on the blacklist too — that is checked first. Or a protection module dropped it, which happens before the whitelist. The bogon filter is the exception: it honours the whitelist |
| A blacklisted address still gets through | The connection was already established; the list only affects new ones |
| Nothing changed after saving | Saving stages. It goes live on [Apply]({{ '/architecture/' | relative_url }}) |
| The editor names a line number | That line is not a valid address or CIDR; the message says why |
| You allowed too broad a range | Remove it, save, apply. If it already locked you out, do nothing — the window rolls it back |

Need the network a single address belongs to?

```bash
whois 198.51.100.42 | grep -i route
```

Logging blacklist hits is a switch on the [options page]({{ '/features/filters/' | relative_url }});
they appear in the kernel log with an `easywall blacklist:` prefix.
