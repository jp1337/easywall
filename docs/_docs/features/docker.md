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
each. It runs at every apply. It also runs once more on its own, for up to 90
seconds after `easywall-core` starts. That covers the ordinary case: no such
bridge existed at boot, and Docker started after the daemon. Past that
window, or for a bridge added later still, apply again, or add an entry to
`custom_networks`.

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
| **1** | `enabled = true` — the shipped default | yes, Docker publishes them | works | most hosts |
| **2** | `enabled = true`, `published_ports = "filtered"` | only with a **forwarded** [port rule]({{ '/docs/features/ports/' | relative_url }}) | works | one firewall on a container host |
| **3** | `enabled = true`, Docker with `{"iptables": false}` | only with a [port rule]({{ '/docs/features/ports/' | relative_url }}) per port | **needs a masquerade rule you write yourself** | one firewall, one place to look |
| **4** | `enabled = false` — deliberately | no | **no** | a host that runs no containers |

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

### A Docker host with IPv6

A user-defined bridge can carry a ULA `/64` (`fd…::/64`) beside its IPv4
subnet:

```bash
docker network create --ipv6 --subnet 172.20.0.0/16 --subnet fd00:20::/64 mynet
```

Publish the port on both families — `-p 993:993` alone binds `0.0.0.0` only,
`-p [::]:993:993` adds the IPv6 side. Docker needs `ip6tables` enabled in its
own daemon config to write the matching `ip6` DNAT rules; that has been the
default since Docker 27.

Once the bridge exists, detection picks up its IPv6 network the same apply it
picks up the IPv4 one. The forwarded port rules that already open a port for
IPv4 open it for IPv6 too — one rule, two families, no separate IPv6 port rule
to write. A bridge's own `fe80::` link-local address never counts as a
container network, so it never opens anything on its own.

`ipv6.mode = "block"` keeps IPv6 away from containers exactly as it does from
the host: no IPv6 twin renders, no IPv6 deny, no IPv6 exception, whatever the
bridge carries.

Containers on an IPv6 bridge reach the host's own services over IPv6 as they
already do over IPv4 — the input chain's bridge accept covers both families.

### A port published on a bridge gateway is still a published port

`-p 172.17.0.1:53:53` publishes to the bridge, not to the world, so it reads
like a container-only service that the `forward` chain's deny cannot be about.
It is. A container on *another* bridge reaches it DNAT'd into the first one, so
it arrives with its destination inside that bridge and its source outside it.
That is the shape of a packet from the internet, because by then it is one. Such
a port needs a **forwarded** rule like any other published port.

This is the sentence that would have saved the host 2.20.1 came from. Its
resolver was published that way, every container lost DNS at the first apply,
and all fifteen external probes stayed green.

> **A forwarded rule that names sources covers only those sources.** What the
> deny closes is the world *plus* every other bridge. Naming the other bridge
> networks restores the container in them, and is the right rule where only
> containers should reach the port. It does not restore the world: a packet to a
> `0.0.0.0`-published port is DNAT'd into a bridge and meets the deny with the
> same shape. Such a rule is still named in the log, because no source list is
> ever tested against that whole set.

### What the log says at each apply

Since 2.20.1 an apply under `filtered` names every published port no forwarded
rule covers, read from Docker's own DNAT rules in the kernel:

```
53 published on 172.17.0.1 with no forwarded rule: the forward chain drops
everything that reaches it from outside its own bridge — the world, and
containers in another bridge. Give it a port rule with scope "forwarded" —
no sources, or sources naming the other bridge networks if only containers
should reach it — or set docker.published_ports = "open"
```

A publish that remaps the port — `-p 8080:80` — names both numbers, because the
rule has to name the second one. The forward chain runs after Docker's DNAT, so
the packet arrives there carrying `80`, and a rule for `8080` matches nothing:

```
8080 published on 0.0.0.0 reaches the container on 80: no forwarded rule for
80: the forward chain drops everything that reaches it from outside its own
bridge — the world, and containers in another bridge. Give it a port rule with
scope "forwarded" — no sources, or sources naming the other bridge networks if
only containers should reach it — or set docker.published_ports = "open"
```

| | |
|---|---|
| Once per apply, every time | No folding of repeats. The apply you are reading the log of is the one that has to say it |
| A sourced rule is named too | It covers only the sources it lists, which is never the whole set the deny closes — see the callout above |
| **Both families** | Docker's `ip nat` and `ip6 nat` DNAT rules are read, so a port published only on IPv6 is named too |
| No Docker socket, no client library | A host whose Docker has stopped with its rules still loaded is exactly the host this is about |

| Also worth knowing | |
|---|---|
| Nothing in the interface sets this key | Edited in `easywall.toml` only. A press that could take every container off the network, with an acceptance window blind to it, is not a control |
| The [ports page]({{ '/docs/features/ports/' | relative_url }}) reads it instead | A forwarded rule is marked inert there while `published_ports` is `open`. It warns; it does not offer to change the key |
| `filtered` needs `enabled = true` | The combination is refused at startup and on `SIGHUP`, by name, rather than accepted and ignored |
| No detected bridge network means nothing is rendered | The `forward` chain is left exactly as it was, with one warning in the log. A deny with no exceptions beside it would close the host's container traffic entirely |
| The ports page cannot see that state | It marks a forwarded rule inert while `published_ports` is `open`, because that key is one it can read. Whether a bridge was detected is known only in the core, so a forwarded rule under `filtered` with no bridge reads as enforced on that page while nothing is rendered. The log warning is the signal; check it after an apply |
| Container-to-container and outbound are untouched | The deny matches only traffic whose destination is a container address and whose source is not |
| A `routing.networks` peer loses reach to published ports | The deny is evaluated before those CIDR exceptions, so a peer you allowed there still needs a forwarded port rule. Expect this as an outage if `routing.networks` is set |
| IPv6 is filtered like IPv4 | Once a bridge carries a ULA or global IPv6 network, the deny and the forwarded rules cover it. Under `ipv6.mode = "block"` IPv6 never reaches a container |
| A bridge the rules in force do not know is a warning | After `docker network create`, the dashboard and `easywall-core status` say *Docker network … exists and is not in the rules in force* until the next apply |

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

For a published port that answers nothing — a resolver, a syslog receiver — no
reply-based probe can tell *dropped* from *silent*. Read the packet counters
instead: [proving a port that answers nothing]({{ '/docs/features/health/' | relative_url }}#proving-a-port-that-answers-nothing).
