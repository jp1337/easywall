#!/bin/bash

echo "Copy configuration..."
cp config/config.ini config/config.ini.example
newconfig=$(sed "s/username.*/username =/g" config/config.ini.example)
echo "$newconfig" >config/config.ini.example
newconfig=$(sed "s/password.*/password =/g" config/config.ini.example)
echo "$newconfig" >config/config.ini.example

echo "Run easywall..."
/usr/bin/python3 core/easywall.py
