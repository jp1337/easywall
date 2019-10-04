# EasyWall [![Codacy Badge](https://api.codacy.com/project/badge/Grade/3e06b3dc52b34cca839c8848d799d251)](https://www.codacy.com/app/jp1337/easywall?utm_source=github.com&amp;utm_medium=referral&amp;utm_content=jp1337/easywall&amp;utm_campaign=Badge_Grade) [![Maintainability](https://api.codeclimate.com/v1/badges/1da84d05446c7a18aece/maintainability)](https://codeclimate.com/github/jp1337/easywall/maintainability) [![GitHub license](https://img.shields.io/github/license/jp1337/easywall.svg)](https://github.com/jp1337/easywall/blob/master/LICENSE) [![GitHub repo size](https://img.shields.io/github/repo-size/jp1337/easywall.svg)](https://github.com/jp1337/easywall) [![Project Version](https://img.shields.io/badge/release-alpha-red.svg)](https://github.com/jp1337/easywall)

Today **Firewalls** are more important than years before. Hackers and Bots are trying to invade your server. So what to do against it?
Set up a firewall! But which one? **IPTables** is the strongest firewall in **Linux** environments. So we wanted to make IPTables usable by everyone. We created a simple **Web Interface** for IPTables so everyone can install and use it on his / her server.

## Demo

> We are currently setting up a self resetting demo server.*

## Features

- **Blacklist for completely blocking an IPV4 / IPV6 Address**
- **Whitelist to allow complete traffic from a specific IP**
- **Opening TCP and UDP Ports**
- **Opening TCP and UDP Port Ranges**
- **Automatic Reset of Firewall Settings on connection loss or bad firewall settings**

## Install

We provide extensive instructions on our [installation](https://github.com/jp1337/easywall/blob/master/INSTALL.md) page.  
You can find a script for automatic downloading and installation.

## Documentation

Check the **[EasyWall wiki](https://github.com/jp1337/easywall/wiki)**.

## License

EasyWall is GNU General Public License v3.0.

It re-distributes other open-source tools and libraries. Please check its [License](https://github.com/jp1337/easywall/blob/master/LICENSE).

## Screenshots

![Screenshot 1: Home Page on Mobile](http://i.imgur.com/vEneFWK.png)
![Screenshot 2: Main Menu on Mobile](http://i.imgur.com/zxCcPQW.png)
![Screenshot 3: Ports Page Upper Part](http://i.imgur.com/qYjxXNZ.png)
![Screenshot 4: Ports Page Lower Part](http://i.imgur.com/zdN0oRu.png)
![Screenshot 5: Apply Page with Automatic Timeout Reset](http://i.imgur.com/BaWMkZD.png)

## Changelog

### 0.0.4 (2019-10-04)

Features:

- added possibility to apply custom IPTables rules
- full implemented webinterface - old PHP sources are history
- rule changes made in the webinterface are only written temporary into web directory
- rules can be applied in the webinterface
- a lot of code improvements
- this is kind the first "stable" version ready for testing
- I will test this on my webserver a lot, so the next versions will be more stable

Bugfixes:

- too many, I can't count them
- there was a long time since the last version

### 0.0.3 (2019-06-30)

Features:

- added EasyWall-Web using flask
- added old php templates to web
- improved install script a lot and added so many features to it
- simplified code using codacy and code climate
- ICMP Support added after testing on a server of mine
- added a daemon script for running EasyWall-Web
- 404 error page added to web
- for a production use of EasyWall-Web I added uwsgi instead of the small development server of flask
- logout button added to web
- added a password generator script and added it to install script

Bugfixes:

- improved exception handling in several files
- the `.running` file was not deleted properly
- moved the system `os.system` to a single function where security checks can be implemented in the future

### 0.0.2 (2019-06-08)

Features:

- Changed branch master to old python branch
- Renamed old master branch to php-old
- Bumped version
- Changed documentation

Bugfixes:

- Information of the user in install.sh if not running as root or using sudo
- Removed quiet option in install.sh for apt-get and pip3 for better user experience

### 0.0.1 (2019-04-24)

Features:

- Incomplete Rework of Branch php-old
- EasyWall is split in two parts in the new concept
- EasyWall Firewall Core Part running as root user finished
- The New EasyWall will be one part running as root and one part running as easywall user which has access to config files.

Performance:
Bugfixes:
