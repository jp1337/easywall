#!/bin/bash

SCRIPTNAME=$(basename "$0")
SCRIPTPATH=$(dirname "$(readlink -f "$0")")
CONFIGFOLDER="config"
CONFIGFILE="easywall.ini"
STEPS=5
STEP=1

if [ "$EUID" -ne 0 ]; then
    read -r -d '' NOROOT <<EOF
Heya! To install easywall you need to have a privileged user.
So you can try these:

# sudo -H bash ${SCRIPTNAME}
or
# su root -c "{$SCRIPTNAME}"
EOF
    echo "$NOROOT"
    exit 1
fi

# Step 1
echo "" && echo "($STEP/$STEPS) install required operating system packages" && ((STEP++))
apt update
apt -y install python3 python3-pip

# Step 2
echo "" && echo "($STEP/$STEPS) install required python 3 packages using pip" && ((STEP++))
pip3 install -r requirements.txt
pip3 install "$SCRIPTPATH"

# Step 3
echo "" && echo "($STEP/$STEPS) creating configuration file from example" && ((STEP++))
cp "$SCRIPTPATH"/"$CONFIGFOLDER"/"$CONFIGFILE".example "$SCRIPTPATH"/"$CONFIGFOLDER"/"$CONFIGFILE"

# Step 4
echo "" && echo "($STEP/$STEPS) create group for configuration file access" && ((STEP++))
addgroup easywall

# Step 5
echo "" && echo "($STEP/$STEPS) installing systemd process for easywall" && ((STEP++))
function installDaemon() {
    SERVICEFILE="/lib/systemd/system/easywall.service"
    read -r -d '' SERVICECONTENT <<EOF
[Unit]
Description=easywall - software for simple control of Linux firewalls
Wants=network-online.target
After=syslog.target time-sync.target network.target network-online.target

[Service]
ExecStart=/usr/bin/env python3 easywall/__init__.py
KillMode=mixed
KillSignal=SIGINT
WorkingDirectory=${SCRIPTPATH}
Restart=always
RestartSec=10
StandardOutput=none
StandardError=none
SyslogIdentifier=easywall
User=root
Group=easywall

[Install]
WantedBy=multi-user.target
EOF
    echo "$SERVICECONTENT" >$SERVICEFILE
    systemctl daemon-reload
    systemctl enable easywall
}

read -r -n1 -p "Do you want to install easywall as a self-starting daemon? [y,n]" DAEMON
case $DAEMON in
y | Y) printf "\\ninstalling daemon ...\\n" && installDaemon ;;
n | N) printf "\\nnot installing daemon.\\n" ;;
*) printf "\\nnot installing daemon.\\n" ;;
esac

# Finished. Printing Introduction
echo ""
read -r -d '' INTRODUCTION <<EOF
------------------------------
Wow! Wasn't that easy?
You successfully installed easywall on your linux system.

So what now?

If you have installed easywall as a daemon you can type the following commands:
# systemctl start easywall
or
# service easywall start

If you want to run easywall manually you can execute:
# sudo python3 easywall/__init__.py

If you have any questions on starting easywall, just create a new GitHub Issue:
https://github.com/jp1337/easywall/issues/new
EOF

echo "$INTRODUCTION"
