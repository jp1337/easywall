#!/bin/bash

## Dependencies Versions ##
BOOTSTRAP="4.3.1"
FONTAWESOME="4.7.0"
JQUERY="3.3.1"
POPPER="1.14.7"

SCRIPTNAME=$(basename "$0")

if [ "$EUID" -ne 0 ]; then
    read -r -d '' NOROOT <<EOF
Heya! To install EasyWall you need to have a privileged user.
So you can try these:

# sudo -H bash $SCRIPTNAME
or
# su root -c "$SCRIPTNAME"
EOF
    echo "$NOROOT"
    exit 1
fi

SCRIPTPATH="$(
    cd "$(dirname "$0")" || exit 1
    pwd -P
)"
STEPS=11
STEP=1

# Step 1
echo "" && echo "($STEP/$STEPS) Installing required packages for easywall" && ((STEP++))
apt-get -q update
apt-get -qy install python3 python3-pip curl

# Step 2
echo "" && echo "($STEP/$STEPS) Installing Python3 Dependencies using Pip" && ((STEP++))
pip3 install Flask
pip3 install watchdog

# Step 3
echo "" && echo "($STEP/$STEPS) Installing required packages for easywall-web" && ((STEP++))
apt-get -qy install uwsgi uwsgi-plugin-python3 wget unzip

# Step 4
echo "" && echo "($STEP/$STEPS) Creating configuration" && ((STEP++))
cp config/config.ini.example config/config.ini

# Step 5
echo "" && echo "($STEP/$STEPS) Making all scripts executable" && ((STEP++))
chmod +x -- *.sh

# Step 6
echo "" && echo "($STEP/$STEPS) Setting up EasyWall core systemd process" && ((STEP++))
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
Group=easywall

[Install]
WantedBy=multi-user.target
EOF
    touch $SERVICEFILE
    echo "$SERVICECONTENT" >$SERVICEFILE
    systemctl daemon-reload
    systemctl enable easywall
}

read -r -n1 -p "Do you want to install easywall-core as a Daemon? [y,n]" DAEMON
case $DAEMON in
y | Y) printf "\\ninstalling service ...\\n" && installDaemon ;;
n | N) printf "\\nNot installing Daemon.\\n" ;;
*) printf "\\nNot installing Daemon.\\n" ;;
esac

# Step 7
echo "" && echo "($STEP/$STEPS) Installing 3rd Party Products for EasyWall Web" && ((STEP++))
WEBDIR="$SCRIPTPATH/web"
TMPDIR="$WEBDIR/tmp"
mkdir "$TMPDIR" && cd "$TMPDIR" || exit 1

# Bootstrap
wget -q --show-progress "https://stackpath.bootstrapcdn.com/bootstrap/$BOOTSTRAP/css/bootstrap.min.css"
cp "bootstrap.min.css" "$WEBDIR/static/css/"
wget -q --show-progress "https://stackpath.bootstrapcdn.com/bootstrap/$BOOTSTRAP/js/bootstrap.min.js"
cp "bootstrap.min.js" "$WEBDIR/static/js/"

# Font Awesome
wget -q --show-progress "https://fontawesome.com/v$FONTAWESOME/assets/font-awesome-$FONTAWESOME.zip"
unzip -q "font-awesome-$FONTAWESOME.zip"
cp -r "font-awesome-$FONTAWESOME/css/"* "$WEBDIR/static/css/"
cp -r "font-awesome-$FONTAWESOME/fonts/"* "$WEBDIR/static/fonts/"

# JQuery Slim (for Bootstrap)
wget -q --show-progress "https://code.jquery.com/jquery-$JQUERY.slim.min.js"
cp jquery-$JQUERY.slim.min.js "$WEBDIR/static/js/"

# Popper (for Bootstrap)
wget -q --show-progress "https://unpkg.com/popper.js@$POPPER/dist/umd/popper.min.js"
cp popper.min.js "$WEBDIR/static/js/"

cd "$SCRIPTPATH" || exit 1
rm -rf "$TMPDIR"

# Step 8
echo "" && echo "($STEP/$STEPS) Adding easywall-web user" && ((STEP++))
/usr/sbin/adduser --system easywall

# Step 9
echo "" && echo "($STEP/$STEPS) Permission correction for web folder" && ((STEP++))
chown -R easywall:root "$WEBDIR"

# Step 10
echo "" && echo "($STEP/$STEPS) Setting up easywall-web systemd process" && ((STEP++))
function installDaemon() {
    SERVICEFILE="/lib/systemd/system/easywall-web.service"
    INSTALLDIR=$(pwd)/web
    read -r -d '' SERVICECONTENT <<EOF
[Unit]
Description=EasyWall Web - The IPTables Interface WebInterface
Wants=network-online.target
After=syslog.target time-sync.target network.target network-online.target

[Service]
ExecStart=/bin/bash easywall_web.sh
WorkingDirectory=$INSTALLDIR
Restart=always
RestartSec=10
StandardOutput=syslog
StandardError=syslog
SyslogIdentifier=easywall-web
User=easywall
Group=root

[Install]
WantedBy=multi-user.target
EOF
    touch $SERVICEFILE
    echo "$SERVICECONTENT" >$SERVICEFILE
    systemctl daemon-reload
    systemctl enable easywall-web
}

read -r -n1 -p "Do you want to install easywall-web as a Daemon? [y,n]" DAEMON
case $DAEMON in
y | Y) printf "\\ninstalling service ...\\n" && installDaemon ;;
n | N) printf "\\nNot installing Daemon.\\n" ;;
*) printf "\\nNot installing Daemon.\\n" ;;
esac

# Step 11
echo "" && echo "($STEP/$STEPS) Please set a password for EasyWall Web" && ((STEP++))
/usr/bin/python3 core/passwd.py

# Finished. Printing Introduction
echo ""
read -r -d '' INTRODUCTION <<EOF
------------------------------
You successfully installed EasyWall on your System!
Wasn't that easy?

So what now?

If you have installed EasyWall as a Daemon you simply have to type:
# systemctl start easywall
# systemctl start easywall-web
or
# service easywall start
# service easywall-web start

If you want to run easywall manually you can enter:
# (sudo) python3 core/easywall.py

if you want to run easywall-web manually you can enter:
# cd web && ./easywall_web.sh
or for debugging
# cd web && ./easywall_web.sh -d

If you have any questions on starting EasyWall, just create a new GitHub Issue:
https://github.com/jp1337/easywall/issues/new
EOF

echo "$INTRODUCTION"
