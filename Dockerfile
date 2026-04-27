# ── Stage 1: Build ──────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

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
FROM alpine:3.23

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
RUN mkdir -p /run/easywall /var/lib/easywall /var/log/easywall /etc/easywall/ssl && \
    chown root:easywall /run/easywall && chmod 750 /run/easywall && \
    chown -R easywall:easywall /var/lib/easywall /var/log/easywall /etc/easywall

# Supervisor config
COPY docker/supervisord.conf /etc/supervisord.conf

EXPOSE 12227

VOLUME ["/etc/easywall", "/var/lib/easywall", "/var/log/easywall"]

ENTRYPOINT ["/sbin/tini", "--"]
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisord.conf", "-n"]
