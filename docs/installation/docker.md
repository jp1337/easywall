---
layout: default
title: Docker Installation
description: Run easywall with Docker Compose — pre-built images on GHCR, Docker Hub, and Quay.io. network_mode host, NET_ADMIN capability.
---

# Docker Installation

Pre-built easywall container images are published to **three public registries** on every release. You don't need to build the image yourself — pull and run.

## Container Registries

The same multi-arch image (`linux/amd64` and `linux/arm64`) is mirrored to all three registries simultaneously by CI. Pick whichever is closest to your geography or already integrated with your tooling — they are byte-for-byte identical.

| Registry | Image reference | Notes |
|---|---|---|
| GitHub Container Registry | `ghcr.io/jp1337/easywall` | Recommended default — fastest pulls in CI |
| Docker Hub | `docker.io/kermit1337/easywall` | Most familiar location for many users |
| Quay.io | `quay.io/jp1337/easywall` | Useful when Docker Hub rate limits hurt |

All three are public — no authentication required for pulls.

## Tag Scheme

easywall maintains a stable four-tag scheme so you can pick exactly the cadence that fits your environment:

| Tag | When it moves | Use it for |
|---|---|---|
| `:latest` | Updated only on tagged releases (`v*.*.*`) | **Production.** Stable, signed-off versions |
| `:vX.Y.Z` | Pinned forever — never reused | **Pinning to a specific version** (e.g. `v2.3.0`) |
| `:edge` | Updated after every successful build on `main` | **Demo / development tracking** — picks up unreleased features |
| `:sha-<commit>` | Pinned forever — one tag per commit | **Rollback / debugging** — reproduce a specific commit |

The public demo at the easywall website uses `:edge` and rolls forward automatically as commits land on `main`. Production deployments should use `:latest` (or pin to a `:vX.Y.Z`) so you decide when to upgrade.

## Quick Start

The repo's `docker-compose.yml` defaults to `:latest`. To start with no further config:

```bash
git clone https://github.com/jp1337/easywall.git
cd easywall
docker compose up -d
```

Open `https://localhost:12227` and complete the first-run wizard.

To pin to a specific version, override the image tag:

```yaml
# docker-compose.override.yml
services:
  easywall:
    image: ghcr.io/jp1337/easywall:v2.3.0
```

Or pull directly without cloning the repo:

```bash
docker pull ghcr.io/jp1337/easywall:latest
docker pull docker.io/kermit1337/easywall:latest
docker pull quay.io/jp1337/easywall:latest
```

## Prerequisites

- Docker Engine 24+ and Docker Compose v2
- Linux host with nftables support
- `NET_ADMIN` capability (available for privileged containers on any kernel)

## Configuration

All persistent data lives in Docker volumes:

| Volume | Path in container | Purpose |
|---|---|---|
| `easywall_data` | `/var/lib/easywall` | Rules JSON, version cache |
| `easywall_logs` | `/var/log/easywall` | Audit log |
| `./config` | `/etc/easywall` | Config files (bind-mounted) |

The `config/` directory is bind-mounted so you can edit `easywall.toml` and
`web.toml` directly. Restart the container after changes:

```bash
docker compose restart
```

## Why `network_mode: host`?

nftables operates on the host kernel's network namespace. Without host networking,
rules applied inside the container would only affect container network traffic,
not the host. The `NET_ADMIN` capability allows the container process to issue
netlink calls that modify host-level nftables tables.

## Using a Custom TLS Certificate

Mount your certificate files and update `config/web.toml`:

```yaml
# docker-compose.yml
volumes:
  - /etc/letsencrypt:/etc/letsencrypt:ro
  - ./config:/etc/easywall
```

```toml
# config/web.toml
[tls]
cert = "/etc/letsencrypt/live/example.com/fullchain.pem"
key  = "/etc/letsencrypt/live/example.com/privkey.pem"
```

## Updating

Pull the latest tag and recreate the container:

```bash
docker compose pull
docker compose up -d
```

For automated updates, [Watchtower](https://containrrr.dev/watchtower/) watches the registry and rolls forward on its own. Recommended schedule: nightly or weekly for production using `:latest`.

<div class="callout callout-info">
  <svg class="callout-icon" viewBox="0 0 20 20" fill="currentColor"><path fill-rule="evenodd" d="M18 10a8 8 0 11-16 0 8 8 0 0116 0zm-7-4a1 1 0 11-2 0 1 1 0 012 0zM9 9a.75.75 0 000 1.5h.253a.25.25 0 01.244.304l-.459 2.066A1.75 1.75 0 0010.747 15H11a.75.75 0 000-1.5h-.253a.25.25 0 01-.244-.304l.459-2.066A1.75 1.75 0 009.253 9H9z" clip-rule="evenodd"/></svg>
  <div class="callout-content">
    <strong>Build provenance</strong>
    <p>Every published image carries OCI labels with the source commit SHA, build timestamp, and license. Inspect with <code>docker buildx imagetools inspect ghcr.io/jp1337/easywall:latest</code>. The CI workflows that publish these images live in <a href="https://github.com/jp1337/easywall/blob/main/.github/workflows/release.yml" target="_blank" rel="noopener"><code>.github/workflows/release.yml</code></a> (releases) and <a href="https://github.com/jp1337/easywall/blob/main/.github/workflows/publish-edge.yml" target="_blank" rel="noopener"><code>publish-edge.yml</code></a> (edge).</p>
  </div>
</div>

## Verifying the Image

The image is signed with the build commit's SHA via the `org.opencontainers.image.revision` label. Verify against the upstream commit:

```bash
docker buildx imagetools inspect \
  --format '{% raw %}{{ index .Manifest.Annotations "org.opencontainers.image.revision" }}{% endraw %}' \
  ghcr.io/jp1337/easywall:latest
```

Compare the printed SHA against the GitHub release tag's commit.
