---
layout: default
title: easywall — nftables firewall with a web interface
description: Linux firewall management with a web interface. Go, nftables via netlink, two-process privilege isolation, and an apply that undoes itself if you get it wrong.
seo: software
---

<div class="docs-hero">
  <div class="docs-hero-glow"></div>

  <div class="docs-hero-pill">
    <span class="docs-hero-pill-dot"></span>
    <span>v{{ site.version }} — Graphite UI · English &amp; German · Language switch</span>
  </div>

  <h1 class="docs-hero-title">
    Your firewall.<br>
    Your rules.<br>
    <span class="docs-hero-accent">No surprises.</span>
  </h1>

  <p class="docs-hero-desc">
    nftables through a web interface that cannot lock you out:
    every apply reverts itself unless you confirm it.
  </p>

  <div class="docs-hero-buttons">
    <a href="{{ '/installation/debian/' | relative_url }}" class="btn btn-primary btn-lg">Get started</a>
    <a href="https://easywall.wdkro.de" class="btn btn-soft btn-lg" target="_blank" rel="noopener">Live demo ↗</a>
    <a href="https://github.com/jp1337/easywall" class="btn btn-ghost btn-lg" target="_blank" rel="noopener">GitHub ↗</a>
  </div>

  <div class="docs-hero-meta">
    <span><strong>Go 1.26</strong></span>
    <span class="docs-hero-meta-sep">·</span>
    <span><strong>GPL-3.0</strong></span>
    <span class="docs-hero-meta-sep">·</span>
    <span><strong>nftables</strong> via netlink</span>
    <span class="docs-hero-meta-sep">·</span>
    <span><strong>Argon2id</strong> auth</span>
  </div>
</div>

<figure class="docs-shot">
  {% include themed-figure.html base="/assets/img/screens/dashboard" ext="png"
     alt="The easywall dashboard: firewall status with acceptance state, pending changes and last apply; tiles counting TCP ports, UDP ports, blacklist, whitelist, custom rules and forwarding; and a recent-activity list." %}
  <figcaption>The dashboard answers one question first: what is this firewall enforcing right now?</figcaption>
</figure>

## The idea

A firewall you edit over the network can lock you out of the machine you are editing
it on. easywall makes that recoverable by default.

{% include themed-figure.html base="/assets/diagrams/apply-flow" ext="svg"
   alt="State machine: editing leads to Staged, applying leads to Live, confirming within the window leads to Confirmed, and letting the window expire leads to Rolled back, from where the staged edits are still available." %}

Editing changes nothing. Applying changes everything — for 120 seconds. If the new
rules cut your connection you cannot click Confirm, and *not* confirming is what
brings the old rules back.

## What it is made of

| | |
|---|---|
| **Two processes** | the web interface runs unprivileged and has no path to the kernel |
| **netlink, not a shell** | rules are Go structs, so there is no command line to inject into |
| **Three rule sets** | Staged, Current, Backup — editing and enforcing are separate |
| **Audit log** | every change records what moved and when, in one JSON object per line |
| **Coexists with Docker** | easywall owns `table inet easywall` and touches nothing else |
| **English and German** | switchable in the interface, including before you sign in |

[How it works →]({{ '/architecture/' | relative_url }})

## The order rules are evaluated

The one thing worth knowing before you write a rule. A whitelisted address reaches
every port; a blacklisted one is dropped before the whitelist is ever consulted.

{% include themed-figure.html base="/assets/diagrams/rule-order" ext="svg"
   alt="Decision flow for an incoming packet: loopback first, then the IPv6 mode, which accepts or drops all IPv6 outright unless it is set to filter; then established connections and ICMP, then protection modules, then Docker bridge networks, then the blacklist which drops, then the whitelist which accepts every port, then open ports, then custom rules, and finally the chain policy which drops." %}

## Where to start

| You want to | Go to |
|---|---|
| Try it without installing anything | [Live demo ↗](https://easywall.wdkro.de) · [Demo mode]({{ '/installation/demo/' | relative_url }}) |
| Install on Debian or Ubuntu — amd64 or arm64 | [.deb package]({{ '/installation/debian/' | relative_url }}) |
| Run it in a container | [Docker]({{ '/installation/docker/' | relative_url }}) |
| Build from source | [Manual install]({{ '/installation/manual/' | relative_url }}) |
| Know what the setup page is asking | [First run]({{ '/installation/first-run/' | relative_url }}) |
| Put rules into the kernel, safely | [Applying rules]({{ '/features/apply/' | relative_url }}) |
| Understand the design | [Architecture]({{ '/architecture/' | relative_url }}) · [Security]({{ '/security/' | relative_url }}) |
| See what is planned | [Roadmap]({{ '/roadmap/' | relative_url }}) |
| Ask a question | [Discord ↗]({{ site.discord }}) · [GitHub issues ↗](https://github.com/jp1337/easywall/issues) |

<section class="docs-cta" markdown="0">
  <div class="docs-cta-glow"></div>
  <h2>Try it before you install it</h2>
  <p>A full interface running against an in-memory mock. Nothing reaches a real firewall.</p>
  <div class="docs-cta-buttons">
    <a href="https://easywall.wdkro.de" class="btn btn-primary btn-lg" target="_blank" rel="noopener">Open the demo ↗</a>
    <a href="{{ '/installation/requirements/' | relative_url }}" class="btn btn-soft btn-lg">Requirements</a>
  </div>
  <p class="docs-cta-credentials">Sign in with <code>demo</code> / <code>demo</code></p>
</section>
