#!/usr/bin/env bash

if [ "$EUID" -ne 0 ] ; then
	echo "ERROR: Please run install.sh as root or with sudo privileges"
	exit 1
fi

# ---------------------------------------------------
# ---------- Define some useful functions -----------
# ---------------------------------------------------

function AddSlash {
	local STR=$1
	local length=${#STR}
	local last_char=${STR:length-1:1}
	[[ $last_char != "/" ]] && STR="$STR/"; :
	echo $STR
}

function RemoveSlash {
	local STR=$1
	local length=${#STR}
	local last_char=${STR:length-1:1}
	[[ $last_char == "/" ]] && STR=${STR:0:length-1}; :
	echo $STR
}

# ---------------------------------------------------
# ------------ Define Install Directories -----------
# ---------------------------------------------------

ROOTDIR=$(pwd)

WEBDIR=$ROOTDIR"/web"
RULESDIR=$ROOTDIR"/rules"
LOGDIR=$ROOTDIR"/log"
IPTABLESDIR=$ROOTDIR"/iptables"
CONFIGDIR=$ROOTDIR"/config"

echo "Please enter the Apache Base Directory followed by [ENTER]:"
read -p "Default [/etc/apache2/]: " APACHEDIR
APACHEDIR=${APACHEDIR:-"/etc/apache2/"}
APACHEDIR=$(AddSlash $APACHEDIR)

APACHEAVAILABLE=$APACHEDIR"sites-available/easywall.conf"
APACHEENABLED=$APACHEDIR"sites-enabled/easywall.conf"

# ---------------------------------------------------
# ---------- Prepare Apache2 Configuration ----------
# ---------------------------------------------------

read -r -d '' APACHECONFIG << EOF
listen 1551
<VirtualHost *:1551>
	DocumentRoot "$WEBDIR"
	ErrorLog \${APACHE_LOG_DIR}/easywall_error.log
	CustomLog \${APACHE_LOG_DIR}/easywall_access.log combined
	
	<Directory "$WEBDIR">
		AuthType Basic
		AuthName "EasyWall"
		AuthUserFile /etc/apache2/.easywall_passwd
		Require valid-user
	</Directory>
</VirtualHost>
EOF

echo "Please enter the username for web login followed by [ENTER]:"
read -p "Default [admin]" USERNAME
USERNAME=${USERNAME:-"admin"}

echo "Please enter the password and repeat it once for web login followed by [ENTER]:"
htpasswd -c $APACHEDIR".easywall_passwd" $USERNAME

touch $APACHEAVAILABLE
echo "$APACHECONFIG" > $APACHEAVAILABLE
ln -sf $APACHEAVAILABLE $APACHEENABLED

if [ ! -f /etc/init.d/apache2 ]; then
    echo "ERROR: Reloading Apache2 Configuration failed! Please reload Apache2 Configuration manually."
else
	/etc/init.d/apache2 reload
fi

# ---------------------------------------------------
# -------- Set Privileges in Root Directory ---------
# ---------------------------------------------------

WWWUSER=$(ps -ef | egrep '(httpd|apache2|apache)' | grep -v `whoami` | grep -v root | head -n1 | awk '{print $1}')

chown -R $WWWUSER:$WWWUSER $WEBDIR
chmod -R 0700 $WEBDIR

chown -R $WWWUSER:$WWWUSER $RULESDIR
chmod -R 0700 $RULESDIR

chown -R $WWWUSER:$WWWUSER $LOGDIR
chmod -R 0700 $LOGDIR

chown -R $WWWUSER:$WWWUSER $IPTABLESDIR
chmod -R 0700 $IPTABLESDIR

chown root:root $IPTABLESDIR"/apply"
chown root:root $IPTABLESDIR"/timer"
chmod 6755 $IPTABLESDIR"/apply"
chmod 6755 $IPTABLESDIR"/timer"

chown -R $WWWUSER:$WWWUSER $CONFIGDIR
chmod -R 0700 $CONFIGDIR

exit 0
