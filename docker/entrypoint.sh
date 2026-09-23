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

# /var/lib/easywall is root's; the web process keeps its state in web/, the one
# directory there it can write. Until 2.22 the whole directory was 0770 and
# shared, and the web user could replace the core's files and plant links in
# their place: root then wrote through them (last_apply) or restored them at
# boot (rules.json). The same shape and the same order as debian/postinst:
# the mode first, so from that line on only root changes what is in it, then
# what an older volume left behind —
#   - a link is removed and never followed; nothing easywall writes is one,
#   - the web's own files move into web/, if the web user owns them, built
#     here and renamed in rather than written into web/ in place,
#   - a panic marker not owned by root is removed: only the core makes one,
#     and one the web user made would leave the firewall down at every start,
#   - an entry that is not a regular file is removed: the core never makes
#     one here,
#   - anything else not owned by root is replaced by a root-owned copy, whose
#     owner could otherwise go on writing it in place whatever the
#     directory's mode.
# web/ first of all: a chown through a link named web would hand its target
# to the web user.
DATA=/var/lib/easywall
chown root:easywall "$DATA" 2>/dev/null || warn "could not set the owner of $DATA"
chmod 0750 "$DATA" 2>/dev/null || warn "could not set the mode of $DATA"
if [ -L "$DATA/web" ] || { [ -e "$DATA/web" ] && [ ! -d "$DATA/web" ]; }; then
    warn "$DATA/web was not a directory; removed"
    rm -f "$DATA/web"
fi
mkdir -p "$DATA/web" 2>/dev/null || warn "could not create $DATA/web"
chown easywall:easywall "$DATA/web" 2>/dev/null || warn "could not set the owner of $DATA/web"
chmod 0700 "$DATA/web" 2>/dev/null || warn "could not set the mode of $DATA/web"
for p in "$DATA"/* "$DATA"/.[!.]* "$DATA"/..?*; do
    { [ -e "$p" ] || [ -L "$p" ]; } || continue
    f=${p##*/}
    [ "$f" = web ] && continue
    if [ -L "$p" ]; then
        warn "$p was a link; removed, not followed"
        rm -f "$p"
        continue
    fi
    case "$f" in
        totp_replay.json|passkeys.json|passkeys.json.corrupt|version_cache.json|telemetry.json)
            if [ -f "$p" ] && [ "$(stat -c %U "$p")" = easywall ]; then
                if [ -e "$DATA/web/$f" ] || [ -L "$DATA/web/$f" ]; then
                    rm -f "$p"
                else
                    # Built in root's directory and renamed into web/, never
                    # written into web/ in place — see debian/postinst.
                    t=
                    if ! { t=$(mktemp "$DATA/.easywall-XXXXXX") &&
                           install -m 0600 -o easywall -g easywall "$p" "$t" &&
                           mv -fT "$t" "$DATA/web/$f" && rm -f "$p"; }; then
                        rm -f "${t:-}"
                        warn "could not move $p into $DATA/web"
                    fi
                fi
                continue
            fi ;;
    esac
    if [ "$(stat -c %u "$p")" = 0 ]; then
        continue
    elif [ "$f" = panic ]; then
        warn "$p was not made by the core; removed, panic mode is not engaged"
        rm -rf "$p"
    elif [ ! -f "$p" ]; then
        warn "$p was not a regular file, which the core never makes here; removed"
        rm -rf "$p"
    else
        warn "$p was not owned by root; replaced by a root-owned copy. Review it"
        if ! { t=$(mktemp "$DATA/.easywall-XXXXXX") && cat "$p" > "$t" && mv -f "$t" "$p"; }; then
            warn "could not give $p to root"
        fi
    fi
done

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

# Said once, here, so an operator reading `docker logs` is not left to wonder
# why `easywall-core health` reports the self-test as unavailable here on a
# container that is otherwise ok. RunSelftest builds a private network namespace
# and sends real packets through a real table to prove that the rules easywall
# writes filter what they claim to — that needs CAP_SYS_ADMIN, and this image
# asks for NET_ADMIN and nothing else, deliberately: a container that already
# shares the host's network and can create namespaces is root on the host by
# another name. Health treats the missing proof as `unprovable` rather than
# `degraded`, so this costs the container nothing — being unable to prove
# something is not the same as it being broken.
# "unless you granted it" and not a check of the capability set, because there
# is nothing in this image to check it with — no capsh, and /proc/self/status
# reports the entrypoint's own bounding set rather than what easywall-core will
# hold. easywall-core makes that read for itself, in its own process, which is
# where it is answerable: 2.20.1's describeProof reports "unavailable here"
# when CAP_SYS_ADMIN is absent and "never recorded" when it is present and
# nobody has run the proof. An operator who adds SYS_ADMIN and runs
# `easywall-core selftest` by hand gets a real proof, and this line would
# otherwise be stale the moment they do.
warn "the rule self-test needs CAP_SYS_ADMIN, which this image does not ask for: unless you granted it, health reports the proof as unavailable here, which is not a failure."

exec "$@"
