#!/bin/bash

### INSTALL REQUIREMENTS ###
apt-get install python3 #python3-pip

### CREATE CONFIGURATION ###
cp config/config.ini.example config/config.ini

### CREATE LOG DIR ###
mkdir log