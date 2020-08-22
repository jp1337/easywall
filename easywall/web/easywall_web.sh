#!/bin/bash

# CONFIGFOLDER="config"
# CONFIGFILE="web.ini"
# SCRIPTPATH=$(dirname "$(readlink -f "$0")")
# CONFIGPATH="$SCRIPTPATH"/../"$CONFIGFOLDER"
# BINDIP=$(awk -F "=" '/bindip/ {print $2}' "$CONFIGPATH"/"$CONFIGFILE")
# BINDPORT=$(awk -F "=" '/bindport/ {print $2}' "$CONFIGPATH"/"$CONFIGFILE")
DEBUG=false

function print_help() {
    read -r -d '' HELP <<EOF
easywall Web
usage: easywall_web.sh [option]
Options and arguments (and corresponding environment variables):
-d     : start in debugging mode, print error messages and stack-traces (also --debug)
-h     : print this help message and exit (also --help)
EOF

    echo "$HELP"
}

while true; do
    case "$1" in
    -d | --debug)
        DEBUG=true
        shift
        ;;
    --)
        shift
        break
        ;;
    -h | --help)
        print_help
        exit 0
        ;;
    *) break ;;
    esac
done

if [[ $DEBUG == true ]]; then
    /usr/bin/env python3 -m easywall.web
else
    uwsgi config/web.ini
fi
