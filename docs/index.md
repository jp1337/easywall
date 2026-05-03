---
layout: default
title: Home
description: easywall — Linux firewall management with a web interface. Built on Go, nftables, and two-process isolation for security-first design.
---

<div class="docs-hero">
  <div class="docs-hero-glow"></div>

  <div class="docs-hero-pill">
    <span class="docs-hero-pill-dot"></span>
    <span>v2.3 — Aurora UI · Demo mode · Live validation</span>
  </div>

  <h1 class="docs-hero-title">
    Your firewall.<br>
    Your rules.<br>
    <span class="docs-hero-accent">No surprises.</span>
  </h1>

  <p class="docs-hero-desc">
    Linux firewall management with a web interface — built for 2026.
    Go, nftables via direct netlink, two-process privilege isolation,
    Argon2id auth. Security problems addressed at the root.
  </p>

  <div class="docs-hero-buttons">
    <a href="{{ '/installation/debian/' | relative_url }}" class="btn btn-primary btn-lg">Get Started</a>
    <a href="{{ '/installation/demo/' | relative_url }}" class="btn btn-soft btn-lg">Try the demo</a>
    <a href="https://github.com/jp1337/easywall" class="btn btn-ghost btn-lg" target="_blank" rel="noopener">GitHub ↗</a>
  </div>

  <div class="docs-hero-meta">
    <span><strong>Go 1.25</strong></span>
    <span class="docs-hero-meta-sep">·</span>
    <span><strong>GPL-3.0</strong></span>
    <span class="docs-hero-meta-sep">·</span>
    <span><strong>nftables</strong> via netlink</span>
    <span class="docs-hero-meta-sep">·</span>
    <span><strong>Argon2id</strong> auth</span>
  </div>
</div>

<section class="docs-why">

## Why easywall exists

<div class="docs-why-grid">

  <div class="docs-why-card">
    <div class="docs-why-card-icon docs-why-icon-problem">
      <svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd"/></svg>
    </div>
    <h3>The problem</h3>
    <p>Linux servers and developer laptops remain high-value targets, and many hosting providers offer no upstream firewall. Configuring nftables by hand is error-prone — one wrong rule and you're locked out of your own server. Tools that simplify this are scarce, often unmaintained, and historically not built with security as the primary goal.</p>
  </div>

  <div class="docs-why-card">
    <div class="docs-why-card-icon docs-why-icon-history">
      <svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm.75-13a.75.75 0 00-1.5 0v5c0 .2.08.39.22.53l3 3a.75.75 0 101.06-1.06L10.75 9.69V5z" clip-rule="evenodd"/></svg>
    </div>
    <h3>The backstory</h3>
    <p>The original easywall (Python/Flask/iptables) was archived in 2022 after a CVE. Two structural decisions caused that vulnerability: the entire stack ran as a single root process, and rule application went through subprocess calls to <code>iptables</code> with user-controlled arguments. A flaw anywhere in the web layer was a flaw in the firewall.</p>
  </div>

  <div class="docs-why-card">
    <div class="docs-why-card-icon docs-why-icon-solution">
      <svg viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M10 18a8 8 0 100-16 8 8 0 000 16zm3.857-9.809a.75.75 0 00-1.214-.882l-3.483 4.79-1.88-1.88a.75.75 0 10-1.06 1.061l2.5 2.5a.75.75 0 001.137-.089l4-5.5z" clip-rule="evenodd"/></svg>
    </div>
    <h3>The rewrite</h3>
    <p>This Go rewrite addresses both root causes. Two processes with structural privilege isolation: the web UI runs unprivileged, the firewall daemon runs as root, and they speak only a typed JSON protocol over a Unix socket. Rules go to nftables via direct netlink — no <code>nft</code> subprocess in the apply path, no shell to escape.</p>
  </div>

</div>

<p class="docs-why-tagline">
  <strong>easywall fills this gap</strong> for everyone who wants a sane firewall without becoming an nftables expert — built so that a vulnerability in the web layer cannot reach the kernel.
</p>

</section>

<section class="docs-features-section">

## What you get

<div class="features-grid">
  <div class="feature-card">
    <span class="fc-icon">🔗</span>
    <h3>nftables via netlink</h3>
    <p>Direct kernel API — no subprocess, no shell injection risk.</p>
  </div>
  <div class="feature-card">
    <span class="fc-icon">🔄</span>
    <h3>Two-step activation</h3>
    <p>Apply rules, confirm over SSH — auto-rollback on timeout.</p>
  </div>
  <div class="feature-card">
    <span class="fc-icon">🐳</span>
    <h3>Docker coexistence</h3>
    <p>Own table <code>inet easywall</code> — never touches Docker's chains.</p>
  </div>
  <div class="feature-card">
    <span class="fc-icon">🛡️</span>
    <h3>Protection modules</h3>
    <p>SYN flood, port scan, bogon filter, ICMP flood, and more.</p>
  </div>
  <div class="feature-card">
    <span class="fc-icon">📋</span>
    <h3>Audit log</h3>
    <p>Every change recorded with timestamp, action, rule type, and user.</p>
  </div>
  <div class="feature-card">
    <span class="fc-icon">🌍</span>
    <h3>i18n</h3>
    <p>English & German, extensible via JSON locale files.</p>
  </div>
  <div class="feature-card">
    <span class="fc-icon">📦</span>
    <h3>Export / Import</h3>
    <p>Full JSON rule backups — downloadable and re-uploadable.</p>
  </div>
  <div class="feature-card">
    <span class="fc-icon">🧪</span>
    <h3>Demo mode</h3>
    <p>Run the UI in-memory with no daemon — perfect for evaluation.</p>
  </div>
</div>

</section>

<section class="docs-quickstart">

## Get started in 60 seconds

### Debian / Ubuntu

```bash
wget https://github.com/jp1337/easywall/releases/latest/download/easywall_amd64.deb
sudo dpkg -i easywall_amd64.deb && sudo apt-get install -f
xdg-open https://localhost:12227
```

### Docker

```bash
git clone https://github.com/jp1337/easywall.git
cd easywall
docker compose up -d
```

Open `https://localhost:12227` and complete the first-run wizard.

<div class="callout callout-warning">
  <svg class="callout-icon" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M8.485 2.495c.673-1.167 2.357-1.167 3.03 0l6.28 10.875c.673 1.167-.17 2.625-1.516 2.625H3.72c-1.347 0-2.189-1.458-1.515-2.625L8.485 2.495zM10 5a.75.75 0 01.75.75v3.5a.75.75 0 01-1.5 0v-3.5A.75.75 0 0110 5zm0 9a1 1 0 100-2 1 1 0 000 2z" clip-rule="evenodd"/></svg>
  <div class="callout-content">
    <strong>Network mode</strong>
    <p>Docker Compose uses <code>network_mode: host</code> and <code>NET_ADMIN</code> capability so the core can manage nftables on the host kernel. This is required for a host-level firewall.</p>
  </div>
</div>

</section>

<section class="docs-deeper">

## Want to know how it works?

The two-process architecture, the IPC protocol, the three-state rule storage, the acceptance window — every detail lives on the [Architecture]({{ '/architecture/' | relative_url }}) page. If you're evaluating easywall for production, that's the page to read.

For configuration options, see [Configuration]({{ '/configuration/' | relative_url }}). For a hosted demo without installing anything, see [Demo Mode]({{ '/installation/demo/' | relative_url }}).

</section>
