#!/bin/sh
# Prepare /etc/easywall before supervisord starts anything.
#
# The image sets the ownership this needs at build time — and a bind mount
# replaces every bit of it. `docker compose up -d`, the first command in the
# installation guide, mounts ./config over /etc/easywall, so the files arrive
# owned by whoever cloned the repository, in a directory the easywall user
# cannot write and with no ssl/ in it at all. easywall-web needs to write
# web.toml (it generates the session key into it on first start, and the wizard
# and the password page rewrite it) and to create its certificate in ssl_dir.
# It could do neither, so it exited before binding, supervisord restarted it
# for ever, and the container stayed "Up" with a healthcheck that only looked
# at the core's socket:
#
#   ERROR "invalid config" error="no usable session_key, and the generated one
#   could not be saved to /etc/easywall/web.toml (permission denied)"
#   WARN exited: easywall-web (exit status 1; not expected)
#   $ curl -k https://localhost:12227/   → connection refused
#
# This runs as root, before supervisord drops the web process to the easywall
# user, and puts the mounted directory into the same shape the Debian package
# installs. On a bind mount that means the files on the host change owner to the
# container's easywall user — see installation/docker.md.

CONF=/etc/easywall
TEMPLATE=/usr/share/easywall/config

warn() { echo "easywall-entrypoint: $*" >&2; }

# A bind mount of an empty directory brings no configuration with it; a named
# volume is filled from the image and already has both files.
for f in easywall.toml web.toml; do
    if [ ! -f "$CONF/$f" ] && [ -f "$TEMPLATE/$f" ]; then
        cp "$TEMPLATE/$f" "$CONF/$f" || warn "could not install a default $f"
    fi
done

# Root-owned, group easywall: the web user must traverse it to reach its own
# config and must not be able to write in it.
chown root:easywall "$CONF" 2>/dev/null || warn "could not set the owner of $CONF"
chmod 0750 "$CONF" 2>/dev/null || warn "could not set the mode of $CONF"

# web.toml belongs to the web user, which rewrites it.
if [ -f "$CONF/web.toml" ]; then
    chown easywall:easywall "$CONF/web.toml" 2>/dev/null || warn "could not set the owner of web.toml"
    chmod 0600 "$CONF/web.toml" 2>/dev/null || warn "could not set the mode of web.toml"
fi

# easywall.toml is read by the *root* daemon and stays out of the web user's
# reach. A network-facing process able to replace it would defeat the
# two-process split entirely.
if [ -f "$CONF/easywall.toml" ]; then
    chown root:root "$CONF/easywall.toml" 2>/dev/null || warn "could not set the owner of easywall.toml"
    chmod 0600 "$CONF/easywall.toml" 2>/dev/null || warn "could not set the mode of easywall.toml"
fi

# easywall generates its own certificate here and replaces it before it expires,
# so the directory has to belong to the web user.
mkdir -p "$CONF/ssl" 2>/dev/null || warn "could not create $CONF/ssl"
chown easywall:easywall "$CONF/ssl" 2>/dev/null || warn "could not set the owner of $CONF/ssl"
chmod 0750 "$CONF/ssl" 2>/dev/null || warn "could not set the mode of $CONF/ssl"

# Say so here rather than leaving it to be discovered as a restart loop. The two
# things easywall-web cannot start without are a writable web.toml and a
# writable ssl directory.
for path in "$CONF/web.toml" "$CONF/ssl"; do
    if ! su easywall -s /bin/sh -c "test -w '$path'" 2>/dev/null; then
        warn "$path is not writable by the easywall user."
        warn "easywall-web cannot start without it. If /etc/easywall is a"
        warn "read-only mount, make it writable, or set session_key in web.toml"
        warn "and point ssl_dir at a writable directory."
    fi
done

exec "$@"
