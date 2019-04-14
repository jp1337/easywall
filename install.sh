#!/bin/bash

STEPS=4
STEP=1

echo "("$STEP"/"$STEPS") Installing required packages" && ((STEP++))
apt-get install -qq python3 python3-pip
pip3 -q install watchdog

echo "("$STEP"/"$STEPS") Creating configuration" && ((STEP++))
cp config/config.ini.example config/config.ini

echo "("$STEP"/"$STEPS") Making all scripts executable" && ((STEP++))
chmod +x *.sh

echo "("$STEP"/"$STEPS") Setting up systemd process" && ((STEP++))
SERVICEFILE="/lib/systemd/system/easywall.service"
INSTALLDIR=$(pwd)
read -r -d '' SERVICECONTENT << EOF
[Unit]
Description=EasyWall - The IPTables Interface Core
Wants=network-online.target
After=syslog.target time-sync.target network.target network-online.target

[Service]
ExecStart=/usr/bin/python3 core/easywall.py
KillMode=mixed
KillSignal=SIGINT
WorkingDirectory=$INSTALLDIR
Restart=always
RestartSec=10
StandardOutput=syslog
StandardError=syslog
SyslogIdentifier=easywall
User=root
Group=root

[Install]
WantedBy=multi-user.target
EOF
touch $SERVICEFILE
echo "$SERVICECONTENT" > $SERVICEFILE
systemctl daemon-reload
systemctl enable easywall

echo "Finished."
echo "To start EasyWall execute 'systemctl start easywall'"