#!/bin/bash

SCRIPTNAME=$(basename "$0")
SCRIPTPATH=$(dirname "$(readlink -f "$0")")
RED="\\e[31m"
GREEN="\\e[32m"
YELLOW="\\e[33m"
NOCOLOR="\\e[0m"

if [ "$EUID" -ne 0 ]; then
    read -r -d '' NOROOT <<EOF
Heya! To remove easywall you need to have a privileged user.
So you can try these:

# sudo bash ${SCRIPTNAME}
or
# su root -c "{$SCRIPTNAME}"
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

read -r -d '' MANUALSTEPS <<EOF
$GREEN
Successfully uninstalled easywall!
$NOCOLOR
The script removed all automatically installed files and removed the linux daemons.
$RED
Now you have to do two things:
$NOCOLOR
1)  Carefully remove the following packages which have been installed by easywall.
    The packages may be used by other software which has not been installed by linux packages.
$YELLOW
    Packages:
    - python3
    - python3-pip
    - curl
    - uwsgi
    - uwsgi-plugin-python3
    - wget
    - unzip
$NOCOLOR
2)  Remove the installation folder itself using:
$RED    'rm -r $SCRIPTPATH'
EOF
echo -e "$MANUALSTEPS"
