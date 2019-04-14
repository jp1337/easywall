#!/bin/bash

echo "Copy configuration..."
cp config/config.ini config/config.ini.example

echo "Run easywall..."
/usr/bin/python3 core/easywall.py