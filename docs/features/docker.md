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

> **These networks are also what opens the `forward` chain.** Container traffic is
> routed, not addressed to the host, and an empty base chain with `policy drop`
> destroys it at the hook whatever Docker's own chain accepted — until 2.5.0 that
> killed every arrangement below. Traffic with a source *or* destination in these
> networks now crosses it **whatever `routing.mode` says**: anything else would take
> a host's containers off the network on the first upgrade.

> **The bogon filter exempts them.** Bridge ranges are RFC 1918, exactly what that
> module drops. Before the exemption, switching it on silently undid coexistence.

See [firewall filters]({{ '/features/filters/' | relative_url }}), and
[`[routing]`]({{ '/configuration/' | relative_url }}#routing) if this host routes for
some other reason as well.

## Three ways to run them together

| | Setup | Inbound to published ports | Outbound from containers | Good for |
|---|---|---|---|---|
| **1** | `enabled = true` — *recommended* | yes, Docker publishes them | works | most hosts |
| **2** | `enabled = true`, Docker with `{"iptables": false}` | only with a [port rule]({{ '/features/ports/' | relative_url }}) per port | **needs a masquerade rule you write yourself** | one firewall, one place to look |
| **3** | `enabled = false` | no | **no** | a host that runs no containers |

> **Option 3 is "no containers", not "quiet containers".** The `forward` chain stays
> closed, and a container's outbound traffic is routed through it like any other. This
> page used to read as though outbound-only containers were fine here. They are not.

> **Option 2, measured — not assumed.** Docker's NAT *is* iptables, and the setting
> removes it. Against Docker 29.7.2, with a peer reachable only by leaving the bridge:
> stock daemon → one `MASQUERADE` rule, seven `DOCKER` filter rules, peer reachable.
> With `{"iptables": false}` → neither, peer unreachable; packets leave with a
> `172.17.x.x` source and nothing comes back. easywall cannot supply the masquerade
> for you: [custom rules]({{ '/features/custom-rules/' | relative_url }}) go into its
> `input` chain, and this needs a `postrouting` chain in a table of your own.

## Checking it worked

```bash
# easywall's table — rebuilt on every apply
sudo nft list table inet easywall

# Docker's — should be untouched
sudo nft list tables | grep -i docker
```
