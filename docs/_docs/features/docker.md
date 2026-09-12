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
published_ports       = "open" # "filtered": easywall decides who reaches a
                               # published container port — see option 2 below
```

Detection reads the interfaces named `docker*` or `br-*` and takes the CIDR of
each. It runs **when rules are applied**, not continuously — a network created
afterwards needs another apply, or an entry in `custom_networks`.

Entries there are CIDR networks — `172.20.0.0/16`, not a single container address —
with `#` comments allowed. Anything else is refused by name rather than accepted and
silently skipped.

> **These networks are also what opens the `forward` chain.** Container traffic is
> routed, not addressed to the host. An empty base chain with `policy drop`
> destroys it at the hook, whatever Docker's own chain accepted. Until 2.5.0 that
> killed every arrangement below. Traffic with a source *or* destination in these
> networks now crosses it **whatever `routing.mode` says**: anything else would take
> a host's containers off the network on the first upgrade.

> **The bogon filter exempts them.** Bridge ranges are RFC 1918, exactly what that
> module drops. Before the exemption, switching it on silently undid coexistence.

See [firewall filters]({{ '/docs/features/filters/' | relative_url }}), and
[`[routing]`]({{ '/docs/configuration/' | relative_url }}#routing) if this host routes for
some other reason as well.

## Four ways to run them together

| | Setup | Inbound to published ports | Outbound from containers | Good for |
|---|---|---|---|---|
| **1** | `enabled = true` — *recommended* | yes, Docker publishes them | works | most hosts |
| **2** | `enabled = true`, `published_ports = "filtered"` | only with a **forwarded** [port rule]({{ '/docs/features/ports/' | relative_url }}) | works | one firewall on a container host |
| **3** | `enabled = true`, Docker with `{"iptables": false}` | only with a [port rule]({{ '/docs/features/ports/' | relative_url }}) per port | **needs a masquerade rule you write yourself** | one firewall, one place to look |
| **4** | `enabled = false` | no | **no** | a host that runs no containers |

### Option 2 is what replaces a cloud firewall

A published port is routed to the container, not addressed to this host, so it
never reaches the `input` chain and no ordinary port rule can see it. Under
`published_ports = "filtered"` easywall's `forward` chain denies inbound traffic
to a container address, and a port rule with scope `forwarded` is what opens one.
That is the whole list of ports on the host in one place — which is what a
provider firewall was doing for you.

> **Switching it on closes every published port that has no forwarded rule.**
> Write the rules first, then flip the key, then apply once. The 120-second
> acceptance window will not save you here: it proves your own connection, and
> yours arrives on the `input` chain while your containers do not.

| Also worth knowing | |
|---|---|
| Nothing in the interface sets this key | Edited in `easywall.toml` only. A press that could take every container off the network, with an acceptance window blind to it, is not a control |
| The [ports page]({{ '/docs/features/ports/' | relative_url }}) reads it instead | A forwarded rule is marked inert there while `published_ports` is `open`. It warns; it does not offer to change the key |
| `filtered` needs `enabled = true` | The combination is refused at startup and on `SIGHUP`, by name, rather than accepted and ignored |
| No detected bridge network means nothing is rendered | The `forward` chain is left exactly as it was, with one warning in the log. A deny with no exceptions beside it would close the host's container traffic entirely |
| The ports page cannot see that state | It marks a forwarded rule inert while `published_ports` is `open`, because that key is one it can read. Whether a bridge was detected is known only in the core, so a forwarded rule under `filtered` with no bridge reads as enforced on that page while nothing is rendered. The log warning is the signal; check it after an apply |
| Container-to-container and outbound are untouched | The deny matches only traffic whose destination is a container address and whose source is not |
| A `routing.networks` peer loses reach to published ports | The deny is evaluated before those CIDR exceptions, so a peer you allowed there still needs a forwarded port rule. Expect this as an outage if `routing.networks` is set |
| IPv6 is unaffected, before and after | Bridge detection is IPv4-only, so inbound IPv6 to a published container port already meets the drop policy. Use `custom_networks` |

> **Option 4 is "no containers", not "quiet containers".** The `forward` chain stays
> closed, and a container's outbound traffic is routed through it like any other. This
> page used to read as though outbound-only containers were fine here. They are not.

> **Option 3, measured — not assumed.** Docker's NAT *is* iptables, and the setting
> removes it. Against Docker 29.7.2, with a peer reachable only by leaving the bridge:
> stock daemon → one `MASQUERADE` rule, seven `DOCKER` filter rules, peer reachable.
> With `{"iptables": false}` → neither, peer unreachable; packets leave with a
> `172.17.x.x` source and nothing comes back. easywall cannot supply the masquerade
> for you: [custom rules]({{ '/docs/features/custom-rules/' | relative_url }}) go into its
> `input` chain, and this needs a `postrouting` chain in a table of your own.

## Checking it worked

```bash
# easywall's table — rebuilt on every apply
sudo nft list table inet easywall

# Docker's — should be untouched
sudo nft list tables | grep -i docker
```
