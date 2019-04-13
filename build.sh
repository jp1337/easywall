#!/bin/bash

echo "Copy configuration..."
cp config/config.ini config/config.ini.example

echo "Run easywall..."
sudo /usr/bin/python3 easywall.py