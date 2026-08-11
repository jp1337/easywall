# ── Stage 1: Build ──────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache dependencies separately from source
COPY go.mod go.sum ./
RUN go mod download

# Build both binaries with version injection
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-s -w -X github.com/jp1337/easywall/internal/shared.CurrentVersion=${VERSION}" \
      -o /out/easywall-core ./cmd/easywall-core && \
    CGO_ENABLED=0 GOOS=linux go build \
      -ldflags "-s -w -X github.com/jp1337/easywall/internal/shared.CurrentVersion=${VERSION}" \
      -o /out/easywall-web ./cmd/easywall-web

# ── Stage 2: Runtime ─────────────────────────────────────────────────────────
FROM alpine:3.24

# Provenance, set here rather than in one of the workflows, because there are
# three build paths — GoReleaser for a release, publish-edge for :edge, and a
# plain `docker build` for anyone building their own — and only this file is on
# all three. installation/docker.md tells operators to read the revision back
# and compare it against the tag; before this, release images carried no labels
# at all and the instruction could not work.
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.title="easywall" \
      org.opencontainers.image.description="Linux firewall management with a web interface" \
      org.opencontainers.image.source="https://github.com/jp1337/easywall" \
      org.opencontainers.image.url="https://easywall-project.org" \
      org.opencontainers.image.documentation="https://easywall-project.org" \
      org.opencontainers.image.licenses="GPL-3.0-or-later" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"

# nftables for firewall management; supervisor to run both processes; tini for signal handling
RUN apk add --no-cache nftables supervisor tini && \
    addgroup -S easywall && \
    adduser  -S -G easywall easywall

COPY --from=builder /out/easywall-core /usr/sbin/easywall-core
COPY --from=builder /out/easywall-web  /usr/sbin/easywall-web

# Assets served by the web process (relative to WorkingDirectory)
COPY web/     /usr/share/easywall/web/
COPY locales/ /usr/share/easywall/locales/

# Default configs — can be overridden with a bind mount
COPY config/  /etc/easywall/

# Runtime directories
# Ownership mirrors the Debian layout, and for the same reason: /etc/easywall
# holds easywall.toml, which the root core reads. Handing the whole directory to
# the unprivileged web user — as `chown -R easywall:easywall /etc/easywall` did
# — let a network-facing process rewrite the configuration root loads.
RUN mkdir -p /run/easywall /var/lib/easywall /var/log/easywall /etc/easywall/ssl && \
    chown root:easywall /run/easywall     && chmod 750 /run/easywall && \
    chown root:easywall /etc/easywall     && chmod 750 /etc/easywall && \
    chown root:root     /etc/easywall/easywall.toml && chmod 600 /etc/easywall/easywall.toml && \
    chown easywall:easywall /etc/easywall/web.toml  && chmod 600 /etc/easywall/web.toml && \
    chown easywall:easywall /etc/easywall/ssl       && chmod 750 /etc/easywall/ssl && \
    chown -R easywall:easywall /var/lib/easywall /var/log/easywall

# Supervisor config
COPY docker/supervisord.conf /etc/supervisord.conf

EXPOSE 12227

VOLUME ["/etc/easywall", "/var/lib/easywall", "/var/log/easywall"]

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf", "-n"]
