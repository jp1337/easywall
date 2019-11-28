#!/bin/bash

SCRIPTDIR=$(dirname "$0")
cd "$SCRIPTDIR" || exit 1
/usr/bin/env systemctl stop easywall
/usr/bin/env systemctl stop easywall-web
/usr/bin/env git pull
/usr/bin/env systemctl start easywall
/usr/bin/env systemctl start easywall-web
