---
layout: default
title: Custom Rules
description: Raw nftables statements for what the other pages cannot express — and the one character that is refused.
---

# Custom Rules

nftables statements, written by hand, appended to easywall's input chain. This is
the escape hatch for anything the typed pages cannot say: a port open to one
source only, a rate limit on a specific service, a log line you want.

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/custom" ext="png"
     alt="The custom rules page: a monospaced text area holding one nftables statement per line with comments between them, a rule counter beneath it, and context cards warning that a malformed rule can lock you out and explaining that only the statement body is written, not the full add rule command." %}
  <figcaption>Syntax is checked as you type, by the same <code>nft</code> that will apply it.</figcaption>
</figure>

## The shape of the file

One statement per line. `#` comments and the blank lines between groups are kept
exactly as you wrote them — they are part of the file, not stripped on save.

```
# allow Prometheus node-exporter only from the monitoring host
ip saddr 192.0.2.50 tcp dport 9100 accept

# rate-limit inbound DNS queries to mitigate amplification
udp dport 53 limit rate 50/second accept

# log and drop traffic to a legacy admin port
tcp dport 10000 log prefix "legacy-admin: " drop
```

Each line is written into the input chain as `add rule inet easywall input <your
line>`, so write the statement only — no `add rule`, no table, no chain.

## No newlines, no semicolons

`nft` reads both as the end of one command and the start of the next. A statement
carrying either is therefore not a rule but a second command, run by the root
daemon, able to reach tables easywall does not own. Both are refused — on save,
on import, and again inside the core.

> **Demonstrated, not theoretical.** An imported rule containing a newline wrote
> into a neighbouring table, which is precisely what "easywall owns one table and
> never looks at another" says cannot happen. The check is on the *shape* of the
> input rather than on nft's grammar, so it does not depend on a subprocess being
> available or on a wrapper happening to be unbalanced. The reasoning is in
> [Security]({{ '/security/' | relative_url }}).

There is also a ceiling of **256 statements** per check, which is far above any
real rule set and far below what a form body can carry.

## Where they are evaluated

Last — after the ports, before the chain policy drops what is left.

{% include themed-figure.html base="/assets/diagrams/rule-order" ext="svg"
   alt="Decision flow for an incoming packet: loopback first, then the IPv6 mode, then established connections and ICMP, then protection modules, then Docker bridge networks, then the blacklist which drops, then the whitelist which accepts every port, then open ports, then custom rules, and finally the chain policy which drops." %}

Two consequences worth knowing:

- A packet an earlier rule already accepted never reaches your statement. To
  *restrict* something the port rules opened, remove the port rule — a custom
  `drop` after it will not be consulted.
- A packet your statement accepts is one the policy would otherwise have
  dropped, which is the usual reason to write one.

## Checked as you type

The editor sends the text to the core, which runs it past `nft --check` in a
throwaway table and reports the line number of anything it rejects. A valid set
costs one `nft` run; only a rejected one is re-checked line by line, to say which
line it was.

| State | What it means |
|---|---|
| No errors | `nft` parses every statement. It does not mean the rule does what you meant |
| A line number | That statement is not valid nftables syntax; the message is nft's own |
| "Validation unavailable" | The core is not reachable, so nothing was checked — not "everything is fine" |

In [demo mode]({{ '/installation/demo/' | relative_url }}) there is no `nft`
behind the interface, so it reports the checker as unavailable rather than
claiming your rules are valid.

## Nothing happens until you apply

Saving stages the change, like every other page. A statement that passes the
syntax check can still be refused by the kernel at apply time — a chain that does
not exist, a set that is not defined — and an apply that fails rolls back to the
previous rules.

## When it does not work

| Symptom | Cause |
|---|---|
| "contains a newline, which nft reads as the end of the command" | Put each statement on its own line; it is not a formatting preference |
| Valid syntax, no effect | An earlier rule accepted the packet first — see *Where they are evaluated* |
| The apply failed and rolled back | The kernel refused the statement. `journalctl -u easywall-core` has nft's message |
| Comments disappeared | They no longer do. Saving keeps `#` lines and blank lines |
| The syntax check says nothing at all | The core is unreachable, or you are in demo mode |
