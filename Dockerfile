# ── Stage 1: Build ──────────────────────────────────────────────────────────
# Pinned to the patch, and to the same one as go.mod's toolchain line — a test
# compares the two. `golang:1.26-alpine` floated to whatever the newest 1.26 was,
# which is how this image came to be built by a Go version no workflow tested with.
FROM golang:1.27.1-alpine AS builder

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

# nftables for firewall management; supervisor to run both processes; tini for
# signal handling; tzdata so TZ (docker-compose.yml) resolves to an actual
# zone. Alpine carries none of the /usr/share/zoneinfo database by default —
# without this package, Go's time.LoadLocation fails for anything but "UTC"
# and "Local" silently means UTC regardless of what TZ says, which is a TZ
# variable that looks respected and is not.
RUN apk add --no-cache nftables supervisor tini tzdata && \
    addgroup -S easywall && \
    adduser  -S -G easywall easywall

COPY --from=builder /out/easywall-core /usr/sbin/easywall-core
COPY --from=builder /out/easywall-web  /usr/sbin/easywall-web

# Assets served by the web process (relative to WorkingDirectory)
COPY web/     /usr/share/easywall/web/
COPY locales/ /usr/share/easywall/locales/

# Default configs — can be overridden with a bind mount, which is why a pristine
# copy stays here as well: a bind mount of an empty directory brings no
# configuration with it, and the entrypoint installs these into it.
COPY config/*.toml /etc/easywall/
COPY config/*.toml /usr/share/easywall/config/

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
    chown root:easywall /var/lib/easywall && chmod 770 /var/lib/easywall && \
    chown root:easywall /var/log/easywall && chmod 750 /var/log/easywall

# Supervisor config
COPY docker/supervisord.conf /etc/supervisord.conf

# The ownership above is set at build time and a bind mount replaces all of it.
# The entrypoint restores it at start, which is what makes `docker compose up -d`
# with the shipped ./config mount work at all — read its header.
COPY docker/entrypoint.sh /usr/local/bin/easywall-entrypoint
RUN chmod 0755 /usr/local/bin/easywall-entrypoint

EXPOSE 12227

VOLUME ["/etc/easywall", "/var/lib/easywall", "/var/log/easywall"]

ENTRYPOINT ["/sbin/tini", "--", "/usr/local/bin/easywall-entrypoint"]
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf", "-n"]
