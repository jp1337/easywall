#!/bin/bash

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

DEBUG=false

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
