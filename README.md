# easywall

[![GitHub Actions Build](https://img.shields.io/github/workflow/status/jp1337/easywall/pytest)](https://github.com/jp1337/easywall/actions)
[![Coveralls github](https://img.shields.io/coveralls/github/jp1337/easywall)](https://coveralls.io/github/jp1337/easywall)
[![GitHub license](https://img.shields.io/github/license/jp1337/easywall)](https://github.com/jp1337/easywall/blob/master/LICENSE)
[![GitHub repo size](https://img.shields.io/github/repo-size/jp1337/easywall.svg)](https://github.com/jp1337/easywall)
[![Project Version](https://img.shields.io/badge/release-beta-yellow.svg)](https://github.com/jp1337/easywall)
[![Discord Support](https://img.shields.io/discord/333980251921186818)](https://discord.gg/CUj2sFb)
[![Beerpay](https://img.shields.io/beerpay/jp1337/easywall)](https://beerpay.io/jp1337/easywall)

**Firewalls** are becoming increasingly important in today's world. Hackers and automated scripts are constantly trying to **invade your system** and use it for Bitcoin mining, botnets or other things.

To prevent these attacks, you can use a firewall on your system. **IPTables** is the strongest firewall in Linux because it can **filter packets in the kernel** before they reach the application. Using IPTables is **not very easy** for Linux beginners. We have created easywall - the simple **IPTables web interface**. The focus of the software is on easy installation and use.

## Features

- Many **built-in rules** that can be activated by option
- **Logging** of blocked connections
- **IPv6** support
- The activation of the changed rules is done in **two steps**
- **Blacklisting** and **whitelisting** of IP addresses
- **Port Forewarding** through the Firewall
- Opening **TCP** and **UDP** ports and **port ranges**
- **Own IPTables rules** can be managed in the web interface
- Easy to **install** and **upgrade**
- **Ansible role** for advanced users and automation

## Demo

You can view an online version of the program as a demo under the following link:

<https://dev01vt.wdkro.de:12227/> (Certificate is provided by Let's Encrypt)

- **Username**: demo
- **Password**: demo

The online demo version is reset every 30 minutes and all data is deleted. The demo has only the frontend installed without the backend. Configuration changes are therefore not saved or applied.

## Support me

The project easywall was created in a time when I did not have any knowledge about Linux. Today I work as an administrator for Linux systems and would not need this project anymore. Nevertheless it is important to me to support the introduction to Linux and the use and simplification of firewalls.

To continue the project, I need your support! Please donate so that I can continue the project in my spare time. Every dollar counts! You can find a donation link on [GitHub](https://github.com/jp1337/easywall).

## Roadmap for the next releases

The following features will be implemented in one of the next versions. If you need a feature, simply create a GitHub issue, and we'll include it here.

- :x: create a `pydoc` documentation
- :x: finish all `TODO: Docs` documentations
- :x: create a linux / debian package for installation
- :x: improve testing by using multiple os
- :x: first run wizard in the web interface for setting username and password after installing

## Install

We provide extensive instructions on our [installation](https://github.com/jp1337/easywall/blob/master/docs/INSTALL.md) page.
You can find a script for automatic downloading and installation.

## Ansible Role

We are currently working on an Ansible role that will allow easywall to be fully configured with Ansible.

The project can be found under the following link:
**[ansible-role-easywall](https://github.com/jp1337/ansible-role-easywall)**

## Documentation

Check the **[docs folder](https://github.com/jp1337/easywall/tree/master/docs)**.

## License

easywall is GNU General Public License v3.0.

## Screenshots

![Screenshot 1](https://i.imgur.com/eQAHOUc.png)
![Screenshot 2](https://i.imgur.com/N2cdm0h.png)
![Screenshot 3](https://i.imgur.com/pjtJuq5.png)
![Screenshot 4](https://i.imgur.com/SSTPaXO.png)
![Screenshot 5](https://i.imgur.com/EPHUjI3.png)
![Screenshot 6](https://i.imgur.com/X3sdFO3.png)
![Screenshot 7](https://i.imgur.com/5kd2Nql.png)
![Screenshot 8](https://i.imgur.com/jjZTxrV.png)
![Screenshot 9](https://i.imgur.com/owPACSx.png)
