# easywall

[![GitHub Actions Build](https://img.shields.io/github/workflow/status/jp1337/easywall/python-testing)](https://github.com/jp1337/easywall/actions)
[![Coveralls github](https://img.shields.io/coveralls/github/jp1337/easywall)](https://coveralls.io/github/jp1337/easywall)
[![GitHub license](https://img.shields.io/github/license/jp1337/easywall)](https://github.com/jp1337/easywall/blob/master/LICENSE)
[![GitHub repo size](https://img.shields.io/github/repo-size/jp1337/easywall.svg)](https://github.com/jp1337/easywall)
[![Project Version](https://img.shields.io/badge/release-development-red.svg)](https://github.com/jp1337/easywall)
[![Discord Support](https://img.shields.io/discord/333980251921186818)](https://discord.gg/CUj2sFb)
[![Beerpay](https://img.shields.io/beerpay/jp1337/easywall)](https://beerpay.io/jp1337/easywall)

Today **Firewalls** are more important than years before. Hackers and Bots are trying to invade your server. So what is the most effective way to prevent this?
Set up a firewall! But which one? **IPTables** is the strongest firewall in **Linux** environments. So we wanted to make IPTables usable by everyone. We created a simple **Web Interface** for IPTables so everyone can install and use it on his / her server.

## Features

- Configuration of Firewall Rules using a **simple Webinterface**
- **Blacklist** for blocking incoming triffic from a IPV4 / IPV6 Address
- **Whitelist** to allow all incoming network traffic from a specific IP Address
- Allow incoming traffic to a **single TCP or UDP Port**
- Allow incoming Traffic from a **TCP or UDP Port range**
- **Automatic Reset of Firewall Settings** on connection loss or bad firewall settings

## Demo

You can visit our Online Demo on the following URL:

<https://dev01vt.wdkro.de:12227/> (Certificate is valid by Let's Encrypt)

- Username: demo
- Password: demo

The demo is resetting every 30 minutes and the data is erased. The demo only contains the web frontend, so the configuration made is not applied.

## Roadmap for the next release

Most of you noticed that there are a lot of changes made at the moment, but the release status is still in "development" and there are no minor releases. This is because I currently enhance the project until the next release. The reason for this is because I learned a lot of Python 3 coding the last year in my main job. I want to use this knowledge and clean this project up so everyone can use it and there are less bugs than before (there were a lot of bugs). If you want to support me and improve the development speed you can send me a beer on beerpay :beers:

- :heavy_check_mark: create a setup.py and setup.cfg file for publishing
- :heavy_check_mark: create a requirements.txt file with all the requirements
- :heavy_check_mark: create github actions testing and linting
- :heavy_check_mark: implement custom rules feature
- :heavy_check_mark: create unit tests for all classes in easywall folder
- :heavy_check_mark: create unit tests for all classes in web folder
- :heavy_check_mark: rework all classes in easywall folder
- :heavy_check_mark: rework all classes in web folder
- :heavy_check_mark: set up a demo server
- :x: SSL Implementation for web application
- :x: write documentation for installing and uninstalling
- :x: write documentation for development setup

## Roadmap for later version

These are long-term planned features. If you want to have more features listed here you just have to create a issue and we can discuss the feature.

- :x: create a `pydoc` documentation
- :x: finish all `TODO: Docs` documentations
- :x: create a ansible role for easy installing
- :x: fill the `Help` button in the web-app with content

<!-- ## Install

We provide extensive instructions on our [installation](https://github.com/jp1337/easywall/blob/master/INSTALL.md) page.
You can find a script for automatic downloading and installation. -->

<!-- ## Documentation

Check the **[easywall wiki](https://github.com/jp1337/easywall/wiki)**. -->

## License

easywall is GNU General Public License v3.0.

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
