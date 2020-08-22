#!/bin/bash

CONFIGFOLDER="config"
CONFIGFILE="easywall.ini"
SAMPLEFILE="easywall.sample.ini"
CONFIGFILELOG="log.ini"
SAMPLEFILELOG="log.sample.ini"
SERVICEFILE="/lib/systemd/system/easywall.service"
LOGFILE="/var/log/easywall.log"

SCRIPTNAME=$(basename "$0")
SCRIPTSPATH=$(dirname "$(readlink -f "$0")")
HOMEPATH="$(dirname "$SCRIPTSPATH")"

STEPS=7
STEP=1

if [ "$EUID" -ne 0 ]; then
    read -r -d '' NOROOT <<EOF
To install easywall, you need administration rights.
You can use the following commands:

# sudo -H bash ${SCRIPTSPATH}/${SCRIPTNAME}
or
# su root -c "${SCRIPTSPATH}/${SCRIPTNAME}"
EOF
    echo "$NOROOT"
    exit 1
fi

# Step 1
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Install the required programs from the operating system \\e[39m" && ((STEP++))
apt -qqq update
apt -y install python3 python3-pip

# Step 2
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Install the required Python3 packages using pip3 \\e[39m" && ((STEP++))
pip3 install setuptools wheel
pip3 install "${HOMEPATH}"

# Step 3
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

# Step 4
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Create the group under which the software should run \\e[39m" && ((STEP++))
if [ "$(getent group easywall)" ]; then
    echo "The easywall group is already present."
else
    groupadd easywall
    echo "The easywall group was created."
fi

# Step 5
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Create the systemd service \\e[39m" && ((STEP++))
read -r -d '' SERVICECONTENT <<EOF
[Unit]
Description=easywall - software for simple control of Linux firewalls
Wants=network-online.target
After=syslog.target time-sync.target network.target network-online.target

[Service]
ExecStart=python3 -m easywall
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
systemctl daemon-reload
systemctl enable easywall
echo "daemon installed."

# Step 6
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Create the logfile \\e[39m" && ((STEP++))
touch "${LOGFILE}"
echo "logfile created."

# Step 7
echo "" && echo -e "\\e[33m($STEP/$STEPS)\\e[32m Start the services \\e[39m" && ((STEP++))
systemctl restart easywall
echo "daemon started."

# Finished.
echo "" && echo ""
read -r -d '' INTRODUCTION <<EOF
\\e[33m------------------------------\\e[39m
You have successfully installed the easywall core on your Linux system.

For the next steps, please follow our installation instructions on GitHub.
https://github.com/jp1337/easywall/blob/master/docs/INSTALL.md

Daemon Status:

EOF
echo -e "${INTRODUCTION}"
systemctl -l status easywall
