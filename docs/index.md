---
layout: default
title: Home
description: easywall — Linux firewall management with a web interface. Built on Go, nftables, and two-process isolation.
---

<div class="hero">
  <div class="hero-brand">
    <img src="{{ '/assets/img/icon.svg' | relative_url }}" alt="easywall">
    <div class="hero-brand-text">
      <h1>easywall</h1>
      <p class="tagline">Your firewall. Your rules. No surprises.</p>
    </div>
  </div>

  <p class="hero-desc">
    Linux firewall management with a web interface — built for 2026.
    Go, nftables via direct netlink, two-process isolation, Argon2id auth.
    Security problems addressed at the root.
  </p>

  <div class="hero-buttons">
    <a href="{{ '/installation/debian/' | relative_url }}" class="btn btn-primary">Get Started</a>
    <a href="https://github.com/jp1337/easywall" class="btn btn-secondary" target="_blank" rel="noopener">GitHub ↗</a>
  </div>
</div>

## Architecture

<div class="arch-diagram">Browser  ──HTTPS──►  easywall-web   (user: easywall, unprivileged)
                           │
                    Unix socket (mode 0660, group easywall)
                    Typed JSON protocol
                           │
                     easywall-core  (root, CAP_NET_ADMIN only)
                           │
                    nftables kernel (via direct netlink — no nft subprocess)</div>

The web process **never touches the firewall directly**. All changes go through a typed socket protocol to a privileged core daemon. Privilege escalation from the web process is structurally impossible.

## Features

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
    <span class="fc-icon">🌍</span>
    <h3>i18n</h3>
    <p>English & German, extensible via JSON locale files.</p>
  </div>
  <div class="feature-card">
    <span class="fc-icon">📦</span>
    <h3>Export / Import</h3>
    <p>Full JSON rule backups — downloadable and re-uploadable.</p>
  </div>
</div>

## Quick Start

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

## Why easywall in 2026?

Linux servers — and increasingly Linux desktops — remain high-value targets. Many hosting providers offer no upstream firewall, leaving individual machines exposed. The Linux desktop market share is growing, developer laptops run on Linux, and simple firewall tools are scarce.

easywall fills this gap for those who are not nftables experts.

The original easywall (Python/Flask/iptables, v0.3.1) was archived after a CVE. This rewrite addresses the root causes: no subprocess execution, no shared-privilege IPC, no weak password hashing.
