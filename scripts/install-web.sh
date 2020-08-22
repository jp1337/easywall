#!/bin/bash

BOOTSTRAP="4.1.3"
FONTAWESOME="4.7.0"
JQUERY="3.3.1"
POPPER="1.14.3"
CONFIGFOLDER="config"
RULESFOLDER="rules"
SSLFOLDER="ssl"
CONFIGFILE="web.ini"
SAMPLEFILE="web.sample.ini"
CONFIGFILELOG="log.ini"
SAMPLEFILELOG="log.sample.ini"
SERVICEFILE="/lib/systemd/system/easywall-web.service"
SERVICEFILE_EASYWALL="/lib/systemd/system/easywall.service"
CERTFILE="easywall.crt"
LOGFILE="/var/log/easywall.log"

SCRIPTNAME=$(basename "$0")
SCRIPTSPATH=$(dirname "$(readlink -f "$0")")
HOMEPATH="$(dirname "$SCRIPTSPATH")"
WEBDIR="$HOMEPATH/easywall/web"
TMPDIR="$WEBDIR/tmp"

STEPS=10
STEP=1

if [ "$EUID" -ne 0 ]; then
    read -r -d '' NOROOT <<EOF
To install easywall-web, you need administration rights.
You can use the following commands:

# sudo -H bash ${SCRIPTSPATH}/${SCRIPTNAME}
or
# su root -c "${SCRIPTSPATH}/${SCRIPTNAME}"
EOF
    echo "$NOROOT"
    exit 1
fi

# Step 1
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Install the required Python3 packages using pip3 \\e[39m" && ((STEP++))
pip3 install "${HOMEPATH}"

# Step 2
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Create the configuration from the example configuration \\e[39m" && ((STEP++))
if [ -f "${HOMEPATH}/${CONFIGFOLDER}/${CONFIGFILE}" ]; then
    echo -e "\\e[33mThe configuration file is not overwritten because it already exists and adjustments may have been made.\\e[39m"
else
    cp -v "${HOMEPATH}/${CONFIGFOLDER}/${SAMPLEFILE}" "${HOMEPATH}/${CONFIGFOLDER}/${CONFIGFILE}"
fi
if [ -f "${HOMEPATH}/${CONFIGFOLDER}/${CONFIGFILELOG}" ]; then
    echo -e "\\e[33mThe log configuration file is not overwritten because it already exists and adjustments may have been made.\\e[39m"
else
    cp -v "${HOMEPATH}/${CONFIGFOLDER}/${SAMPLEFILELOG}" "${HOMEPATH}/${CONFIGFOLDER}/${CONFIGFILELOG}"
fi

# Step 3
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Create the group under which the software should run \\e[39m" && ((STEP++))
if [ "$(getent group easywall)" ]; then
    echo "The easywall group is already present."
else
    groupadd easywall
    echo "The easywall group was created."
fi

# Step 4
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Download of several libraries required for easywall-web \\e[39m" && ((STEP++))
mkdir "$TMPDIR" && cd "$TMPDIR" || exit 1

# Bootstrap
wget -q --timeout=10 --tries=5 --retry-connrefused --show-progress "https://stackpath.bootstrapcdn.com/bootstrap/$BOOTSTRAP/css/bootstrap.min.css" && cp -v "bootstrap.min.css" "$WEBDIR/static/css/"
wget -q --timeout=10 --tries=5 --retry-connrefused --show-progress "https://stackpath.bootstrapcdn.com/bootstrap/$BOOTSTRAP/js/bootstrap.min.js" && cp -v "bootstrap.min.js" "$WEBDIR/static/js/"

# Font Awesome
wget -q --timeout=10 --tries=5 --retry-connrefused --show-progress "https://fontawesome.com/v$FONTAWESOME/assets/font-awesome-$FONTAWESOME.zip"
unzip -q "font-awesome-$FONTAWESOME.zip"
cp -rv "font-awesome-$FONTAWESOME/css/"* "$WEBDIR/static/css/"
cp -rv "font-awesome-$FONTAWESOME/fonts/"* "$WEBDIR/static/fonts/"

# JQuery Slim (for Bootstrap)
wget -q --timeout=10 --tries=5 --retry-connrefused --show-progress "https://code.jquery.com/jquery-$JQUERY.slim.min.js" && cp -v jquery-$JQUERY.slim.min.js "$WEBDIR/static/js/"

# Popper (for Bootstrap)
wget -q --timeout=10 --tries=5 --retry-connrefused --show-progress "https://cdnjs.cloudflare.com/ajax/libs/popper.js/$POPPER/umd/popper.min.js" && cp -v popper.min.js "$WEBDIR/static/js/"

cd "$HOMEPATH" || exit 1
rm -rf "$TMPDIR"

# Step 5
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Create the application user and add it to the application group. \\e[39m" && ((STEP++))
adduser --system --debug easywall
usermod -g easywall easywall

# Step 6
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Set permissions on files and folders \\e[39m" && ((STEP++))
chown -Rv easywall:easywall "${HOMEPATH}"
chown -Rv easywall:easywall "$WEBDIR"
chown -Rv easywall:easywall "${HOMEPATH}/${CONFIGFOLDER}"
chown -Rv easywall:easywall "${HOMEPATH}/${RULESFOLDER}"
chmod -v 750 "${HOMEPATH}"
chmod -v 750 "${HOMEPATH}/${CONFIGFOLDER}"
chmod -Rv 750 "${HOMEPATH}/${RULESFOLDER}"

# Step 7
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Create the systemd service \\e[39m" && ((STEP++))
read -r -d '' SERVICECONTENT <<EOF
[Unit]
Description=easywall-web - web interface to control the easywall core application.
Wants=network-online.target
After=syslog.target time-sync.target network.target network-online.target

[Service]
ExecStart=/bin/bash easywall/web/easywall_web.sh
WorkingDirectory=${HOMEPATH}
StandardOutput=syslog
StandardError=syslog
SyslogIdentifier=easywall-web
User=easywall
Group=easywall

[Install]
WantedBy=multi-user.target
EOF
echo "${SERVICECONTENT}" >"${SERVICEFILE}"
systemctl daemon-reload
systemctl enable easywall-web
echo "daemon installed."

# Step 8
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Create a self-signed SSL certificate \\e[39m" && ((STEP++))
if [ ! -f "${HOMEPATH}/${SSLFOLDER}/${CERTFILE}" ]; then
    DOMAIN="$(hostname -f)"
    echo "generating passphrase..."
    export PASSPHRASE
    PASSPHRASE=$(
        head -c 500 /dev/urandom | tr -dc a-z0-9A-Z | head -c 128
        echo
    )

    SUBJECT="
C=DE
ST=Berlin
O=easywall
localityName=Berlin
commonName=$DOMAIN
organizationalUnitName=IT
emailAddress=admin@example.com
"
    echo "generating private key..."
    openssl genrsa -des3 -out easywall.key -passout env:PASSPHRASE 4096

    echo "generating certificate sign request..."
    openssl req \
        -new \
        -batch \
        -subj "$(echo -n "$SUBJECT" | tr "\\n" "/")" \
        -key easywall.key \
        -out easywall.csr \
        -passin env:PASSPHRASE

    echo "change des3 private key to rsa private key"
    openssl rsa -in easywall.key -out easywall.key -passin env:PASSPHRASE

    echo "sign certificate sign request using openssl"
    openssl x509 -req -days 3650 -in easywall.csr -signkey easywall.key -out easywall.crt

    echo "moving certificates in place..."
    mv -v easywall.crt "${HOMEPATH}/${SSLFOLDER}/"
    mv -v easywall.key "${HOMEPATH}/${SSLFOLDER}/"
    rm -v easywall.csr
    chown -Rv easywall:easywall "${HOMEPATH}/${SSLFOLDER}/"
    chmod -v 700 "${HOMEPATH}/${SSLFOLDER}"
    chmod -Rv 600 "${HOMEPATH}/${SSLFOLDER}/*"
else
    echo "The certificate already exists and does not need to be created."
fi

# Step 9
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Create the logfile \\e[39m" && ((STEP++))
touch "${LOGFILE}"
echo "logfile created."

# Step 10
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Start the services \\e[39m" && ((STEP++))
if [ -f "${SERVICEFILE_EASYWALL}" ]; then
    systemctl restart easywall
fi
systemctl restart easywall-web
echo "daemon started."

# Finished.
echo "" && echo ""
read -r -d '' INTRODUCTION <<EOF
\\e[33m------------------------------\\e[39m
You have successfully installed the easywall web interface.

For the next steps, please follow our installation instructions on GitHub.
https://github.com/jp1337/easywall/blob/master/docs/INSTALL.md

Daemon Status:

EOF
echo -e "${INTRODUCTION}"
systemctl -l status easywall-web
