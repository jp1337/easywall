#!/bin/bash

SCRIPTNAME=$(basename "$0")
SCRIPTSPATH=$(dirname "$(readlink -f "$0")")

if [ "$EUID" -ne 0 ]; then
    read -r -d '' NOROOT <<EOF
To remove easywall, you need administration rights.
You can use the following commands:

# sudo -H bash ${SCRIPTSPATH}/${SCRIPTNAME}
or
# su root -c "${SCRIPTSPATH}/${SCRIPTNAME}"
EOF
    echo "$NOROOT"
    exit 1
fi

FILE="/lib/systemd/system/easywall.service"
if test -f "$FILE"; then
    systemctl stop easywall.service
    rm $FILE
    systemctl daemon-reload
fi

FILE="/lib/systemd/system/easywall-web.service"
if test -f "$FILE"; then
    systemctl stop easywall-web.service
    rm $FILE
    systemctl daemon-reload
fi

if id "easywall" >/dev/null 2>&1; then
    deluser easywall
fi

# Finished.
echo "" && echo ""
read -r -d '' INSTRUCTIONS <<EOF
\\e[33m------------------------------\\e[39m
easywall was successfully uninstalled!

For the next steps, please follow our uninstallation instructions on GitHub.
https://github.com/jp1337/easywall/blob/master/docs/UNINSTALL.md

EOF
echo -e "${INSTRUCTIONS}"
