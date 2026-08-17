---
layout: default
title: Requirements
description: What the host needs, what it does not, and which install path to take.
---

# Requirements

{% include themed-figure.html base="/assets/diagrams/install-choice" ext="svg"
   alt="Decision tree: just looking leads to demo mode; Debian or Ubuntu leads to the .deb package; already running containers leads to Docker; otherwise build from source." %}

## The host

| | Minimum | |
|---|---|---|
| Kernel | 3.13 | when the nftables netlink API arrived |
| nftables | any | `apt install nftables` |
| init | systemd | for the service units |
| Architecture | amd64 or arm64 | binaries shipped for both |
| RAM | ~32 MB | both processes, idle |
| Disk | ~20 MB | binaries, assets, config |
| Privilege | `CAP_NET_ADMIN` | the core needs it to write rules. The web process does not |

## Tested on

| Distribution | Architectures |
|---|---|
| Debian 12 · Debian 11 | amd64, arm64 |
| Ubuntu 24.04 LTS · 22.04 LTS | amd64, arm64 |
| Raspbian (Debian 12) | arm64 |

Other systemd distributions with nftables — Arch, Fedora, openSUSE — should work but
are not in CI.

## Ports

| Port | Direction | Purpose |
|---|---|---|
| 12227/tcp | inbound | the web interface, HTTPS only |

There is no plaintext port. The two processes talk over a Unix socket, not a port.

## Not required

- Python, Node.js, or any runtime beyond the two binaries
- A database
- A Go toolchain on the target — use the `.deb` or a release binary
- **Outbound internet access.** Fonts, stylesheet and scripts are served by easywall
  itself, so it renders correctly on an air-gapped host. Two things reach out and
  both fail quietly: the update check, which is on and can be switched off, and the
  installation count, which is off until someone switches it on. Both are listed in
  full under [Security]({{ '/docs/security/' | relative_url }})

## Coming from easywall v1

v1 used iptables and YAML; v2 uses nftables and JSON. The rule file cannot be
imported — recreate the rules, or use [export/import]({{ '/docs/features/export-import/' | relative_url }})
once v2 is running.

```bash
sudo systemctl disable --now easywall easywall-web   # stop v1
pip uninstall easywall                               # if installed via pip

# optional: v1's iptables rules are independent of nftables and can stay,
# but if you want them gone
sudo iptables -F && sudo iptables -X
sudo ip6tables -F && sudo ip6tables -X
```

**Next:** [Debian / Ubuntu]({{ '/docs/installation/debian/' | relative_url }}) ·
[Docker]({{ '/docs/installation/docker/' | relative_url }}) ·
[From source]({{ '/docs/installation/manual/' | relative_url }}) ·
[Demo mode]({{ '/docs/installation/demo/' | relative_url }}) ·
[First run]({{ '/docs/installation/first-run/' | relative_url }})
