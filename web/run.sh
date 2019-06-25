#!/bin/bash

DEBUG=false
SCRIPTPATH="$( cd "$(dirname "$0")" ; pwd -P )"
PYTHONPATH=$(dirname $SCRIPTPATH)

while getopts d opt
do
   case $opt in
       d) DEBUG=true;;
   esac
done

if [[ $DEBUG == true ]] ; then
    export PYTHONPATH=../
    python3 app.py
else
    uwsgi --http-socket 127.0.0.1:9000 --need-plugin python3 --wsgi-file app.py --callable app --pythonpath $PYTHONPATH --processes 4 --threads 2
fi
