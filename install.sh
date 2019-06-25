#!/bin/bash

if [ "$EUID" -ne 0 ]; then
    read -r -d '' NOROOT <<EOF
Heya! To install EasyWall you need to have a privileged user.
So you can try these:

# sudo bash install.sh
or
# su root -c "install.sh"
EOF
    echo "$NOROOT"
    exit
fi

STEPS=4
STEP=1

echo "($STEP/$STEPS) Installing required packages" && ((STEP++))
apt-get clean
apt-get update
apt-get -y install python3 python3-watchdog python3-flask uwsgi uwsgi-plugin-python3

echo "($STEP/$STEPS) Creating configuration" && ((STEP++))
cp config/config.ini.example config/config.ini

echo "($STEP/$STEPS) Making all scripts executable" && ((STEP++))
chmod +x -- *.sh

function installDaemon() {
    SERVICEFILE="/lib/systemd/system/easywall.service"
    INSTALLDIR=$(pwd)
    read -r -d '' SERVICECONTENT <<EOF
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
    echo "$SERVICECONTENT" >$SERVICEFILE
    systemctl daemon-reload
    systemctl enable easywall
}

echo "($STEP/$STEPS) Setting up systemd process" && ((STEP++))
read -r -n1 -p "Do you want to install easywall as a Daemon? [y,n]" DAEMON
case $DAEMON in
y | Y) printf "\\ninstalling service ...\\n" && installDaemon ;;
n | N) printf "\\nNot installing Daemon.\\n" ;;
*) printf "\\nNot installing Daemon.\\n" ;;
esac

read -r -d '' INTRODUCTION <<EOF

------------------------------
You successfully installed EasyWall on your System!
Wasn't that easy?

So what now?

If you have installed EasyWall as a Daemon you simply have to type:
# systemctl start easywall
or
# service easywall start

If you want to run easywall manually you can enter:
# (sudo) python3 core/easywall.py

If you have any questions on starting EasyWall, just create a new GitHub Issue:
https://github.com/jp1337/easywall/issues/new
EOF

echo "$INTRODUCTION"
