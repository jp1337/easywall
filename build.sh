#!/bin/bash

echo "Copy configuration..."
cp config/config.ini config/config.ini.example

echo "Run easywall..."
sudo /usr/bin/python3.6 easywall.py