#!/usr/bin/env bash
# Runs easywall-web in demo mode for local review: no easywall-core, no root,
# no nftables, an in-memory mock behind the same HTTP handlers a real
# installation uses.
#
# The other half of this script is the certificate. A mkcert CA, once
# installed with `mkcert -install`, is trusted by Chrome's NSS store — but
# every review session still met a self-signed-certificate interstitial,
# because easywall-web was generating its own certificate that CA had never
# heard of. Worse, the old scratch directory was under /tmp, which is cleared
# on reboot, so the leaf was regenerated on every reboot and silently
# invalidated whatever exception the browser had been given. This script
# fixes both: it keeps its state under $HOME, and it hands easywall-web a
# certificate mkcert's already-trusted CA actually signed, when mkcert is
# available.
#
# It never runs `mkcert -install`. Trusting a CA is a change to the user's own
# trust store, and that is the user's decision alone, not this script's.
set -euo pipefail
cd "$(dirname "$0")/.."

# Never /tmp — see above. $HOME survives a reboot, so a certificate issued
# here still validates the next time this script runs.
DEMO_DIR="${EASYWALL_DEMO_DIR:-$HOME/.local/share/easywall-demo}"
SSL_DIR="$DEMO_DIR/ssl"
CERT="$SSL_DIR/cert.pem"
KEY="$SSL_DIR/key.pem"
mkdir -p "$SSL_DIR"

# A browser certificate exception is per-origin — scheme, host *and* port —
# so changing this address on the self-signed fallback path costs a fresh
# click-through; it does not on the mkcert path, because the CA is trusted
# regardless of which port the leaf was issued for.
ADDR="${EASYWALL_DEMO_ADDR:-127.0.0.1:12227}"

# Prefer mkcert, but only if a CA root actually exists — `mkcert -CAROOT`
# prints a directory whether or not anything was ever installed into it.
tls_block=""
if command -v mkcert >/dev/null 2>&1 &&
	caroot="$(mkcert -CAROOT 2>/dev/null)" && [ -f "$caroot/rootCA.pem" ]; then
	# Reissue only when there is no leaf yet or the one on disk has expired —
	# a script that reissues on every run invalidates any exception already
	# granted, which is the exact bug this script exists to end.
	if [ ! -f "$CERT" ] || ! openssl x509 -checkend 0 -noout -in "$CERT" >/dev/null 2>&1; then
		mkcert -cert-file "$CERT" -key-file "$KEY" 127.0.0.1 localhost >/dev/null
	fi
	tls_block="

[tls]
cert = \"$CERT\"
key  = \"$KEY\""
else
	echo "no trusted mkcert CA found: easywall-web will serve its own" \
		"self-signed certificate, and the browser will show an interstitial." \
		"Run 'mkcert -install' yourself once to remove it for good — this" \
		"script will not do that for you." >&2
fi

# admin / ui-check-password-2026 is what scripts/ui-check.mjs signs in with.
# An earlier hand-written demo config used the username "ui-check" instead and
# that mismatch cost a debugging detour, so this hash is for "admin" and
# nothing else. Generated once with internal/web.HashPassword — argon2id is
# salted, so this literal decodes and verifies but is not itself a secret
# worth rotating.
PASSWORD_HASH='$argon2id$v=19$m=65536,t=3,p=4$HdRCEmi7PNvF2ckkTaBMeA$OkVq3v321f2Ws3zFI482qActF+E5FOwnOEc0z8otgeU'

cat >"$DEMO_DIR/web.toml" <<EOF
bind_addr    = "$ADDR"
socket_path  = "/nonexistent.sock"
ssl_dir      = "$SSL_DIR"
data_dir     = "$DEMO_DIR"
language     = "en"
session_key  = "$(openssl rand -hex 32)"
demo_mode    = true
update_check = false
username     = "admin"
password     = "$PASSWORD_HASH"
telemetry    = false
totp_secret  = ""
recovery_codes = []$tls_block
EOF

[ -x bin/easywall-web ] || go build -o bin/easywall-web ./cmd/easywall-web

echo "easywall-web (demo mode) on https://$ADDR — state in $DEMO_DIR" >&2
exec bin/easywall-web -config "$DEMO_DIR/web.toml"
