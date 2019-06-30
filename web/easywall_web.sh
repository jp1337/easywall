#!/bin/bash

SCRIPTPATH="$(
    cd "$(dirname "$0")" || exit 1
    pwd -P
)"
PYTHONPATH=$(dirname "$SCRIPTPATH")/core
CONFIGPATH=$(dirname "$SCRIPTPATH")/config
BINDIP=$(awk -F "=" '/bindip/ {print $2}' "$CONFIGPATH"/config.ini)
BINDPORT=$(awk -F "=" '/bindport/ {print $2}' "$CONFIGPATH"/config.ini)
DEBUG=false

function print_help() {
    read -r -d '' HELP <<EOF
EasyWall Web
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
    export PYTHONPATH=$PYTHONPATH
    /usr/bin/python3 app.py
else
    uwsgi --http-socket "$BINDIP:$BINDPORT" --need-plugin python3 --wsgi-file app.py --callable app --pythonpath "$PYTHONPATH" --processes 4 --threads 2
fi
