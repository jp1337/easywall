#!/bin/bash

echo "Copy configuration..."
cp config/config.ini config/config.ini.example
newconfig=$(sed "s/username.*/username =/g" config/config.ini.example)
echo "$newconfig" >config/config.ini.example
newconfig=$(sed "s/password.*/password =/g" config/config.ini.example)
echo "$newconfig" >config/config.ini.example
newconfig=$(sed "s/bindip.*/bindip = 127.0.0.1/g" config/config.ini.example)
echo "$newconfig" >config/config.ini.example
newconfig=$(sed "s/bindport.*/bindport = 12227/g" config/config.ini.example)
echo "$newconfig" >config/config.ini.example
newconfig=$(sed "s/sha.*/sha = 0/g" config/config.ini.example)
echo "$newconfig" >config/config.ini.example
newconfig=$(sed "s/date.*/date = 0/g" config/config.ini.example)
echo "$newconfig" >config/config.ini.example
newconfig=$(sed "s/timestamp.*/timestamp = 0/g" config/config.ini.example)
echo "$newconfig" >config/config.ini.example

echo "Run easywall..."
/usr/bin/python3 core/easywall.py
