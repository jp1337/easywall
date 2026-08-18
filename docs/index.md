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
    <span>A firewall for your server that<em> cannot lock you out.</em></span>
  </div>

  <h1 class="docs-hero-title">
    Your firewall.<br>
    Your rules.<br>
    <span class="docs-hero-accent">No surprises.</span>
  </h1>

  <p class="docs-hero-desc">
    easywall is a web interface for the Linux nftables firewall. Every apply
    reverts itself after 120 seconds unless you confirm it — so editing your
    firewall over the network can never cut you off. Runs on a home server, a
    Raspberry Pi, or a rented VPS.
  </p>

  <div class="docs-hero-buttons">
    <a href="{{ '/docs/installation/debian/' | relative_url }}" class="btn btn-primary btn-lg">Install</a>
    <a href="https://easywall.wdkro.de" class="btn btn-soft btn-lg">Live demo ↗</a>
    <a href="{{ '/docs/' | relative_url }}" class="btn btn-ghost btn-lg">Documentation</a>
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
     alt="The easywall dashboard: firewall status with acceptance state, pending changes and last apply; tiles counting TCP and UDP ports, blacklist, whitelist, custom rules and forwarding; recent-activity list." %}
  <figcaption>The dashboard answers one question first: what is this firewall enforcing right now?</figcaption>
</figure>

<section class="docs-landstrip">
  <div class="docs-cardgrid">
    <div class="docs-card">
      <h2>Debian / Ubuntu</h2>
      <p class="mono">sudo dpkg -i easywall_amd64.deb</p>
      <p>amd64 and arm64, one package, a systemd unit and HTTPS built in.</p>
      <a href="{{ '/docs/installation/debian/' | relative_url }}">Install →</a>
    </div>
    <div class="docs-card">
      <h2>Docker</h2>
      <p class="mono">docker compose up -d</p>
      <p>Lives in its own nftables table and touches nothing else on the host.</p>
      <a href="{{ '/docs/installation/docker/' | relative_url }}">Run it →</a>
    </div>
    <div class="docs-card">
      <h2>From source</h2>
      <p class="mono">make build && sudo make install</p>
      <p>Go 1.26, no runtime dependencies beyond the kernel.</p>
      <a href="{{ '/docs/installation/manual/' | relative_url }}">Build it →</a>
    </div>
  </div>
</section>

## A firewall you keep your hands on

Everything lives on your own box: ports TCP/UDP, blacklist and whitelist, port
forwarding, custom nftables rules, protection modules, a theft-proof audit log.
No cloud, no account, no telemetry you did not agree to.

* Ports — single or range, with per-rule SSH brute-force routing
* Blacklist & whitelist — IPv4, IPv6, CIDR, evaluated before any port rule
* Port forwarding — NAT redirects with protocol selection
* Custom rules — raw nftables, syntax-checked before it is ever applied
* Docker coexistence — owns `table inet easywall`, touches nothing else
* English & German, light & dark

[What it can do →]({{ '/docs/' | relative_url }})

<section class="docs-cta" markdown="0">
  <div class="docs-cta-glow"></div>
  <h2>Try it before you install it</h2>
  <p>A full interface running against an in-memory mock. Nothing reaches a real firewall.</p>
  <div class="docs-cta-buttons">
    <a href="https://easywall.wdkro.de" class="btn btn-primary btn-lg" target="_blank" rel="noopener">Open the demo ↗</a>
    <a href="{{ '/docs/installation/requirements/' | relative_url }}" class="btn btn-soft btn-lg">Requirements</a>
  </div>
  <p class="docs-cta-credentials">Sign in with <code>demo</code> / <code>demo</code></p>
</section>

## Built as open source, for 2026

Go, GPL-3.0, nftables via netlink. The apply can never lock you out. Take a
look at the [architecture]({{ '/docs/architecture/' | relative_url }}) and the
[security model]({{ '/docs/security/' | relative_url }}). Find us on
[Discord]({{ site.discord }}) and [GitHub](https://github.com/jp1337/easywall).