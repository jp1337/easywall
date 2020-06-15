#!/bin/bash

SCRIPTNAME=$(basename "$0")
SCRIPTSPATH=$(dirname "$(readlink -f "$0")")
HOMEPATH="$(dirname "$SCRIPTSPATH")"
CONFIGFOLDER="config"
CONFIGFILE="easywall.ini"
SAMPLEFILE="easywall.sample.ini"
SERVICEFILE="/lib/systemd/system/easywall.service"
STEPS=6
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
echo "" && echo -e "\e[33m($STEP/$STEPS)\e[32m install required operating system packages \e[39m" && ((STEP++))
apt -qqq update
apt -y install python3 python3-pip

# Step 2
echo "" && echo -e "\e[33m($STEP/$STEPS)\e[32m install required python 3 packages using pip \e[39m" && ((STEP++))
pip3 install "${HOMEPATH}"

# Step 3
echo "" && echo -e "\e[33m($STEP/$STEPS)\e[32m creating configuration file from example \e[39m" && ((STEP++))
if [ -f "${HOMEPATH}/${CONFIGFOLDER}/${CONFIGFILE}" ]; then
    echo -e "\e[33mThe configuration file is not overwritten because it already exists and adjustments may have been made.\e[39m"
else
    cp -v "${HOMEPATH}/${CONFIGFOLDER}/${SAMPLEFILE}" "${HOMEPATH}/${CONFIGFOLDER}/${CONFIGFILE}"
fi

# Step 4
echo "" && echo -e "\e[33m($STEP/$STEPS)\e[32m create group for configuration file access \e[39m" && ((STEP++))
if [ "$(getent group easywall)" ]; then
    echo "The easywall group is already present."
else
    groupadd easywall
    echo "The easywall group was created."
fi

# Step 5
echo "" && echo -e "\e[33m($STEP/$STEPS)\e[32m installing systemd process for easywall \e[39m" && ((STEP++))
read -r -d '' SERVICECONTENT <<EOF
[Unit]
Description=easywall - software for simple control of Linux firewalls
Wants=network-online.target
After=syslog.target time-sync.target network.target network-online.target

[Service]
ExecStart=/usr/bin/env python3 -m easywall
KillMode=mixed
KillSignal=SIGINT
WorkingDirectory=${HOMEPATH}
StandardOutput=syslog
StandardError=syslog
SyslogIdentifier=easywall
User=root
Group=easywall

[Install]
WantedBy=multi-user.target
EOF
echo "${SERVICECONTENT}" >"${SERVICEFILE}"
/usr/bin/systemctl daemon-reload
/usr/bin/systemctl enable easywall
echo "daemon installed."

# Step 6
echo "" && echo -e "\e[33m($STEP/$STEPS)\e[32m starting easywall process \e[39m" && ((STEP++))
/usr/bin/systemctl restart easywall
echo "daemon started."

# Finished.
echo "" && echo ""
read -r -d '' INTRODUCTION <<EOF
\e[33m------------------------------\e[39m
You have successfully installed the easywall core on your Linux system.

For the next steps, please follow our installation instructions on GitHub.
https://github.com/jp1337/easywall/blob/master/docs/INSTALL.md

Daemon Status:

EOF
echo -e "${INTRODUCTION}"
/usr/bin/systemctl -l status easywall
