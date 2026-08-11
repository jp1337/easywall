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

## Three ways to run them together

| | Setup | Container ports reachable from outside | Good for |
|---|---|---|---|
| **1** | `enabled = true` — *recommended* | yes, Docker publishes them | Most hosts |
| **2** | Docker with `{"iptables": false}` | **only if you add a port rule** | One firewall, one place to look |
| **3** | `enabled = false`, no published ports | no | Containers that only need outbound |

> **Option 2 has a sharp edge.** With `iptables: false` in
> `/etc/docker/daemon.json`, `-p 80:80` no longer opens anything. Every container
> port you want reachable needs its own [port rule]({{ '/features/ports/' | relative_url }}).
> Outbound still works through Docker's NAT.

## Checking it worked

```bash
# easywall's table — rebuilt on every apply
sudo nft list table inet easywall

# Docker's — should be untouched
sudo nft list tables | grep -i docker
```
