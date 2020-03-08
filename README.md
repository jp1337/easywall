# easywall

[![GitHub Actions Build](https://img.shields.io/github/workflow/status/jp1337/easywall/python-testing)](https://github.com/jp1337/easywall/actions)
[![CodeCov Coverage](https://img.shields.io/codecov/c/gh/jp1337/easywall)](https://codecov.io/gh/jp1337/easywall)
[![GitHub license](https://img.shields.io/github/license/jp1337/easywall)](https://github.com/jp1337/easywall/blob/master/LICENSE)
[![GitHub repo size](https://img.shields.io/github/repo-size/jp1337/easywall.svg)](https://github.com/jp1337/easywall)
[![Project Version](https://img.shields.io/badge/release-development-red.svg)](https://github.com/jp1337/easywall)
[![Discord Support](https://img.shields.io/discord/333980251921186818)](https://discord.gg/CUj2sFb)
[![Beerpay](https://img.shields.io/beerpay/jp1337/easywall)](https://beerpay.io/jp1337/easywall)

Today **Firewalls** are more important than years before. Hackers and Bots are trying to invade your server. So what is the most effective way to prevent this?
Set up a firewall! But which one? **IPTables** is the strongest firewall in **Linux** environments. So we wanted to make IPTables usable by everyone. We created a simple **Web Interface** for IPTables so everyone can install and use it on his / her server.

## Roadmap for the next release

Most of you noticed that there are a lot of changes made at the moment, but the release status is still in "development" and there are no minor releases. This is because I currently enhance the project until the next release. The reason for this is because I learned a lot of Python 3 coding the last year in my main job. I want to use this knowledge and clean this project up so everyone can use it and there are less bugs than before (there were a lot of bugs). If you want to support me and improve the development speed you can send me a beer on beerpay :beers:

- :heavy_check_mark: create a setup.py and setup.cfg file for publishing
- :heavy_check_mark: create a requirements.txt file with all the requirements
- :heavy_check_mark: create github actions testing and linting
- :x: create unit tests for all classes in easywall folder
- :x: create unit tests for all classes in web folder
- :x: rework all classes in easywall folder
- :x: rework all classes in web folder
- :x: write documentation for installing and uninstalling
- :x: write documentation for contributing

## Roadmap for later version

- :x: create a `pydoc` documentation
- :x: finish all `TODO: Docs` documentations
- :x: set up a demo server
- :x: create a ansible role for easy installing

<!-- ## Demo

> We are currently setting up a self resetting demo server.\* -->

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

Check the **[easywall wiki](https://github.com/jp1337/easywall/wiki)**.

## License

easywall is GNU General Public License v3.0.

It re-distributes other open-source tools and libraries. Please check its [License](https://github.com/jp1337/easywall/blob/master/LICENSE).

## Screenshots

![Screenshot 1: Home Page on Mobile](https://i.imgur.com/vEneFWK.png)
![Screenshot 2: Main Menu on Mobile](https://i.imgur.com/zxCcPQW.png)
![Screenshot 3: Ports Page Upper Part](https://i.imgur.com/qYjxXNZ.png)
![Screenshot 4: Ports Page Lower Part](https://i.imgur.com/zdN0oRu.png)
![Screenshot 5: Apply Page with Automatic Timeout Reset](https://i.imgur.com/BaWMkZD.png)

## DEV Environment

To create a development environment you have to execute the following command after cloning the repository:

```bash
python3 setup.py develop
```
