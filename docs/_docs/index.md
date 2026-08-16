---
layout: default
title: easywall — how it works and where things are
description: The easywall documentation — install, configure, and understand the nftables firewall that cannot lock you out.
permalink: /docs/
sitemap: false
---

The old home page's exposition now lives here: the idea, what the product is
made of, and the rule order are the orientation you need before or after
installing.

## The idea

A firewall you edit over the network can lock you out of the machine you are editing it on. easywall makes that recoverable by default.

{% include themed-figure.html base="/assets/diagrams/apply-flow" ext="svg"
   alt="State machine: editing leads to Staged, applying leads to Live, confirming within the window leads to Confirmed, and letting the window expire leads to Rolled back, from where the staged edits are still available." %}

Editing changes nothing. Applying changes everything — for 120 seconds. If the new rules cut your connection you cannot click Confirm, and *not* confirming is what brings the old rules back.

## What it is made of

| | |
|---|---|
| **Two processes** | the web interface runs unprivileged and has no path to the kernel |
| **netlink, not a shell** | rules are Go structs, so there is no command line to inject into |
| **Three rule sets** | Staged, Current, Backup — editing and enforcing are separate |
| **Audit log** | every change records what moved and when, in one JSON object per line |
| **Coexists with Docker** | easywall owns `table inet easywall` and touches nothing else |
| **English and German** | switchable in the interface, including before you sign in |

[How it works →]({% link _docs/architecture.md %})

## The order rules are evaluated

The one thing worth knowing before you write a rule. A whitelisted address reaches every port; a blacklisted one is dropped before the whitelist is ever consulted.

{% include themed-figure.html base="/assets/diagrams/rule-order" ext="svg"
   alt="Decision flow for an incoming packet: loopback first, the IPv6 mode, established connections and ICMP, protection modules, Docker bridge networks, blacklist, whitelist, open ports, custom rules, chain policy drops." %}

## Where to start

| You want to | Go to |
|---|---|
| Install on Debian or Ubuntu — amd64 or arm64 | [.deb package]({% link _docs/installation/debian.md %}) |
| Run it in a container | [Docker]({% link _docs/installation/docker.md %}) |
| Build from source | [Manual install]({% link _docs/installation/manual.md %}) |
| Try it without installing anything | [Live demo](https://easywall.wdkro.de) · [Demo mode]({% link _docs/installation/demo.md %}) |
| Know what the setup page is asking | [First run]({% link _docs/installation/first-run.md %}) |
| Put rules into the kernel, safely | [Applying rules]({% link _docs/features/apply.md %}) |
| Understand the design | [Architecture]({% link _docs/architecture.md %}) · [Security]({% link _docs/security.md %}) |
| See what is planned | [Roadmap]({% link _docs/roadmap.md %}) |
| Contribute or ask | [Contributing]({% link _docs/contributing.md %}) · [Discord]({{ site.discord }}) |