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
    <a href="https://easywall.wdkro.de" class="btn btn-soft btn-lg" target="_blank" rel="noopener">Live demo ↗</a>
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

<section class="docs-why" markdown="0">
  <h2>Why easywall exists</h2>

  <p class="docs-section-lead">
    Configuring a Linux firewall is one of those tasks that sits awkwardly
    between "trivially easy" and "production-critical". The tools are
    powerful, the syntax is dense, and the consequences of getting it
    wrong range from "service unreachable" to "operator locked out
    permanently". easywall exists to make this everyday operation safe,
    visible, and reversible — without hiding what's actually happening
    in the kernel.
  </p>

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
</section>

<section class="docs-design" markdown="0">
  <h2>Built for production, not just convenience</h2>

  <p class="docs-section-lead">
    Most firewall-management tools optimise for speed of getting started.
    easywall optimises for what happens <em>after</em> — when the rules
    are running on a server you cannot easily reach, and a wrong move
    means a four-hour drive to a data centre or a long support ticket
    with your hosting provider. Every design choice in easywall traces
    back to that scenario.
  </p>

  <div class="docs-design-grid">

    <div class="docs-design-row">
      <div class="docs-design-row-num">01</div>
      <div class="docs-design-row-body">
        <h3>Privilege isolation by design</h3>
        <p>The web UI process runs as an unprivileged user. It has no kernel access, no <code>CAP_NET_ADMIN</code>, no way to invoke <code>nft</code>. A vulnerability in template rendering, form parsing, or session handling cannot escalate to firewall manipulation, because the privileges live in a separate process the web UI talks to over a typed Unix socket.</p>
      </div>
    </div>

    <div class="docs-design-row">
      <div class="docs-design-row-num">02</div>
      <div class="docs-design-row-body">
        <h3>No subprocess in the apply path</h3>
        <p>Rules are constructed as Go structs and applied via the <a href="https://pkg.go.dev/github.com/google/nftables" target="_blank" rel="noopener">google/nftables</a> netlink library — direct kernel API, no shell to escape, no argv to inject into. The only exception is custom rules, which are passed to <code>nft -f -</code> over stdin (still no argv injection) and run only in the privileged core.</p>
      </div>
    </div>

    <div class="docs-design-row">
      <div class="docs-design-row-num">03</div>
      <div class="docs-design-row-body">
        <h3>Reversible by inaction</h3>
        <p>Applying rules starts an <strong>acceptance timer</strong>. If you don't explicitly confirm within the window (default 120 seconds, configurable), the previous rules are restored automatically. Misconfigure a rule that locks you out of SSH? Wait. The lockout reverts. This is the <code>commit confirmed</code> pattern from Cisco IOS, made the default workflow.</p>
      </div>
    </div>

    <div class="docs-design-row">
      <div class="docs-design-row-num">04</div>
      <div class="docs-design-row-body">
        <h3>Three-state rule storage</h3>
        <p>Rules live in three parallel sets at all times: <strong>Current</strong> (running in the kernel), <strong>Staged</strong> (your editor changes), and <strong>Backup</strong> (last-known-good). You see exactly what's pending, what's live, and what you'd revert to. No accidental kernel writes, no "did I save that?" doubt.</p>
      </div>
    </div>

    <div class="docs-design-row">
      <div class="docs-design-row-num">05</div>
      <div class="docs-design-row-body">
        <h3>Auditable</h3>
        <p>Every administrative action — every rule save, apply, rollback, options change, settings change — is recorded in <code>audit.log</code> with a timestamp, the action type, and the user. The web UI exposes the last 200 entries, the file on disk goes back forever. Pair it with your SIEM or just <code>tail -f</code> it on shell.</p>
      </div>
    </div>

    <div class="docs-design-row">
      <div class="docs-design-row-num">06</div>
      <div class="docs-design-row-body">
        <h3>Coexists with Docker</h3>
        <p>easywall manages its own nftables table (<code>inet easywall</code>) and never touches Docker's chains. You can detect Docker bridge networks automatically and whitelist them, or manage them manually. The two firewall systems do not interfere — neither writes into the other's tables.</p>
      </div>
    </div>

  </div>
</section>

<section class="docs-features-section" markdown="0">
  <h2>What you get</h2>
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
      <p>English &amp; German, extensible via JSON locale files.</p>
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

<section class="docs-stack" markdown="0">
  <h2>The technical foundation</h2>

  <p class="docs-section-lead">
    easywall is opinionated about its tech choices because each one has
    a security or operability story behind it. There are no surprising
    dependencies and nothing chosen for fashion.
  </p>

  <div class="docs-stack-grid">
    <div class="docs-stack-item">
      <div class="docs-stack-label">Language</div>
      <div class="docs-stack-value">Go 1.25</div>
      <div class="docs-stack-note">Statically linked binaries, no runtime dependencies on the target machine. Memory-safe.</div>
    </div>
    <div class="docs-stack-item">
      <div class="docs-stack-label">Firewall API</div>
      <div class="docs-stack-value">nftables (netlink)</div>
      <div class="docs-stack-note">Direct kernel interface via google/nftables. No <code>nft</code> subprocess in the apply path.</div>
    </div>
    <div class="docs-stack-item">
      <div class="docs-stack-label">Auth</div>
      <div class="docs-stack-value">Argon2id</div>
      <div class="docs-stack-note">OWASP-recommended password hash. The original easywall used a weak hash.</div>
    </div>
    <div class="docs-stack-item">
      <div class="docs-stack-label">Sessions</div>
      <div class="docs-stack-value">gorilla/sessions</div>
      <div class="docs-stack-note">HMAC-signed cookies, HttpOnly + Secure + SameSite=Lax flags, 7-day lifetime.</div>
    </div>
    <div class="docs-stack-item">
      <div class="docs-stack-label">CSRF</div>
      <div class="docs-stack-value">Origin-check</div>
      <div class="docs-stack-note">Go 1.25 <code>net/http.CrossOriginProtection</code>. Validates Origin and Sec-Fetch-Site on every state-changing request.</div>
    </div>
    <div class="docs-stack-item">
      <div class="docs-stack-label">UI</div>
      <div class="docs-stack-value">Tailwind v4 + DaisyUI 5.5</div>
      <div class="docs-stack-note">Modern component library, custom Aurora theme. Hand-rolled CSS retired entirely.</div>
    </div>
    <div class="docs-stack-item">
      <div class="docs-stack-label">Interactions</div>
      <div class="docs-stack-value">HTMX 2.0</div>
      <div class="docs-stack-note">Live validation, auto-save, audit log filter — all without a JavaScript framework or build pipeline.</div>
    </div>
    <div class="docs-stack-item">
      <div class="docs-stack-label">License</div>
      <div class="docs-stack-value">GPL-3.0</div>
      <div class="docs-stack-note">Free software, copyleft. Source available on GitHub. Patches and bug reports welcome.</div>
    </div>
  </div>
</section>

<section class="docs-cta" markdown="0">
  <div class="docs-cta-glow"></div>
  <h2>Ready to try it?</h2>
  <p>
    Pick the path that fits your environment. The Quickstart covers
    Debian / Ubuntu, Docker, and a manual source install. The demo
    runs without root or any nftables dependency — explore the entire
    UI on any machine, including your laptop.
  </p>
  <div class="docs-cta-buttons">
    <a href="{{ '/installation/requirements/' | relative_url }}" class="btn btn-primary btn-lg">Quickstart guide</a>
    <a href="https://easywall.wdkro.de" class="btn btn-soft btn-lg" target="_blank" rel="noopener">Live demo ↗</a>
    <a href="{{ '/architecture/' | relative_url }}" class="btn btn-ghost btn-lg">Read the architecture →</a>
  </div>
  <p class="docs-cta-credentials">
    Demo credentials: <code>demo</code> / <code>demo</code> &nbsp;·&nbsp; resets every 6 hours
  </p>
</section>
