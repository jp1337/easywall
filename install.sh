#!/bin/bash

echo "Installing required packages..."
apt-get install -qq python3 #python3-pip

echo "Creating configuration..."
cp config/config.ini.example config/config.ini

echo "Making all scripts executable..."
chmod +x *.sh

echo "Finished."