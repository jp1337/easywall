#!/bin/bash

SCRIPTPATH="$( cd "$(dirname "$0")" || exit 1 ; pwd -P )"
PYTHONPATH=$(dirname "$SCRIPTPATH")/core
CONFIGPATH=$(dirname "$SCRIPTPATH")/config
BINDIP=$(awk -F "=" '/bindip/ {print $2}' "$CONFIGPATH"/config.ini)
BINDPORT=$(awk -F "=" '/bindport/ {print $2}' "$CONFIGPATH"/config.ini)

while getopts d opt
do
   case $opt in
       d) DEBUG=true;;
       *) DEBUG=false;;
   esac
done

if [[ $DEBUG == true ]] ; then
    export PYTHONPATH=$PYTHONPATH
    /usr/bin/python3 app.py
else
    uwsgi --http-socket "$BINDIP:$BINDPORT" --need-plugin python3 --wsgi-file app.py --callable app --pythonpath "$PYTHONPATH" --processes 4 --threads 2
fi
