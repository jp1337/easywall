---
layout: default
title: Docker Coexistence
description: easywall owns one nftables table and touches nothing else, so Docker's chains survive an apply.
---

# Docker Coexistence

easywall v1 flushed iptables and took Docker's chains with it. v2 cannot: it owns
one table and never looks at another.

{% include themed-figure.html base="/assets/diagrams/docker-coexist" ext="svg"
   alt="easywall creates, flushes and replaces only table inet easywall. It never touches table ip docker or any other table." %}

An apply flushes and rebuilds `table inet easywall`. `DOCKER`, `DOCKER-USER` and
`DOCKER-ISOLATION` live elsewhere and are not read, written or deleted.

## Turning it on

```toml
[docker]
enabled               = true   # detect Docker bridge interfaces
allow_bridge_networks = true   # accept traffic from the detected CIDRs
custom_networks       = []     # anything detection misses
```

Detection reads the interfaces named `docker*` or `br-*` and takes the CIDR of
each. It runs **when rules are applied**, not continuously — a network created
afterwards needs another apply, or an entry in `custom_networks`.

> **The networks listed here are also what opens the `forward` chain.** Container
> traffic is routed, not addressed to the host: out of the bridge, through the
> forward hook, on to the world — and back the same way for a published port,
> which Docker translates before easywall's chain sees it. easywall's `forward`
> chain has `policy drop`, and until 2.5.0 it had no rules at all, which is not
> neutrality: an empty base chain drops everything at its hook regardless of what
> Docker's own chain has already accepted. Every arrangement below was dead.
> Traffic with a source *or* a destination in one of these networks now crosses
> that chain, and does so **whatever `routing.mode` is set to** — anything else
> would take a host's containers off the network the first time it upgraded
> without discovering the new key. See
> [firewall filters]({{ '/features/filters/' | relative_url }}), and
> [`[routing]`]({{ '/configuration/' | relative_url }}#routing) if this host routes
> for some other reason as well.

> **The bogon filter and bridge networks get along.** Bridge ranges are RFC 1918,
> which is exactly what that module drops — so it reads the networks listed here and
> exempts them. Before it did, switching the filter on silently undid Docker
> coexistence: the packet was dropped long before the rule allowing it was reached.
> See [firewall filters]({{ '/features/filters/' | relative_url }}).

## Three ways to run them together

| | Setup | Container ports reachable from outside | Good for |
|---|---|---|---|
| **1** | `enabled = true` — *recommended* | yes, Docker publishes them | Most hosts |
| **2** | `enabled = true`, Docker with `{"iptables": false}` | **only if you add a port rule** — and outbound needs a masquerade rule of your own | One firewall, one place to look |
| **3** | `enabled = false` | no — and containers reach nothing outbound either | A host that runs no containers |

> **Option 3 is "no containers", not "quiet containers".** `enabled = false` leaves
> the `forward` chain closed, and a container's outbound traffic is routed through
> it like any other. It used to read as though outbound-only containers were fine
> under this setting; they were not, and under any of the other settings either.

> **Option 2 has two sharp edges.** With `iptables: false` in
> `/etc/docker/daemon.json`, `-p 80:80` no longer opens anything: every container
> port you want reachable needs its own
> [port rule]({{ '/features/ports/' | relative_url }}).
>
> And **outbound stops working too**. This page used to say it kept working
> "through Docker's NAT" — but Docker's NAT *is* iptables, and the setting removes
> it. Measured against Docker 29.7.2 with a peer reachable only by leaving the
> bridge: with the daemon as it comes, one `MASQUERADE` rule and seven `DOCKER`
> filter rules, peer reachable; with `{"iptables": false}`, none of either, peer
> unreachable. A container's packets leave with a `172.17.x.x` source and nothing
> comes back. If you choose this option you have to supply the masquerade
> yourself — easywall cannot do it for you, because
> [custom rules]({{ '/features/custom-rules/' | relative_url }}) are appended to
> its `input` chain and this needs a `postrouting` chain in a table of your own.

## Checking it worked

```bash
# easywall's table — rebuilt on every apply
sudo nft list table inet easywall

# Docker's — should be untouched
sudo nft list tables | grep -i docker
```
