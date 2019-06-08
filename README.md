# EasyWall [![Codacy Badge](https://api.codacy.com/project/badge/Grade/3e06b3dc52b34cca839c8848d799d251)](https://www.codacy.com/app/jp1337/easywall?utm_source=github.com&amp;utm_medium=referral&amp;utm_content=jp1337/easywall&amp;utm_campaign=Badge_Grade) [![Maintainability](https://api.codeclimate.com/v1/badges/ce2e46f2877468bdf256/maintainability)](https://codeclimate.com/github/KingJP/EasyWall/maintainability) [![GitHub license](https://img.shields.io/github/license/jp1337/easywall.svg)](https://github.com/jp1337/easywall/blob/master/LICENSE) [![GitHub repo size](https://img.shields.io/github/repo-size/jp1337/easywall.svg)](https://github.com/jp1337/easywall) [![Project Version](https://img.shields.io/badge/release-alpha-red.svg)](https://github.com/jp1337/easywall)

Today **Firewalls** are more important than years before. Hackers and Bots are trying to invade your server. So what to do against it?
Set up a firewall! But which one? **IPTables** is the strongest firewall in **Linux** environments. So we wanted to make IPTables usable by everyone. We created a simple **Web Interface** for IPTables so everyone can install and use it on his / her server.

## Demo

> We are currently setting up a self resetting demo server.*

## Features

 - **Modern Mobile First UI**
 - **Multiple Options for your Firewall Rules.**
 - **Blacklist for completely blocking an IPV4 / IPV6 Address**
 - **Whitelist to allow complete traffic from a specific IP**
 - **TCP and UDP Ports**
 - **Port Ranges**
 - **Special Firewall Rules for SSH Port**
 - **Build-In Rules for Dropping bad Packets**
 - **Two Factor Confirmation of Firewall Settings**

## Install

We provide extensive instructions on our [installation](https://github.com/KingJP/EasyWall/blob/master/INSTALL.md) page.  
You can find a script for automatic downloading and installation.

## Documentation

Check the **[EasyWall wiki](https://github.com/kingjp/easywall/wiki)**.

## License

netdata is GNU General Public License v3.0.

It re-distributes other open-source tools and libraries. Please check its [License](https://github.com/kingjp/easywall/blob/master/LICENSE).

## Screenshots

![Screenshot 1: Home Page on Mobile](http://i.imgur.com/vEneFWK.png)
![Screenshot 2: Main Menu on Mobile](http://i.imgur.com/zxCcPQW.png)
![Screenshot 3: Ports Page Upper Part](http://i.imgur.com/qYjxXNZ.png)
![Screenshot 4: Ports Page Lower Part](http://i.imgur.com/zdN0oRu.png)
![Screenshot 5: Apply Page with Automatic Timeout Reset](http://i.imgur.com/BaWMkZD.png)

## Changelog

### 1.0.6 (2017-05-02)

Features:

- Extended Configuration to set new Bash Variables
- Added Bootstrap Toggle Button for Settings and Ports
- Error Handling with PHP-Error from [JosephLenton/PHP-Error](https://github.com/JosephLenton/PHP-Error)

Performance:
Bugfixes:

- Reduced Host Information to only print Server Information
