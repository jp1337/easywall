# How to install easywall

The installation of easywall can be done with the following simple steps.

Please check first whether your operating system is already supported by easywall.

Please read the text carefully to avoid errors.

## Check requirements

- SSH access to the system
- root access on the system either as user or via sudo

## Supported operating systems

We currently only support Debian based operating systems like Debian or Ubuntu.

## Install easywall

If the requirements apply to your operating system, you can perform the following installation steps.

## Installation by Package

- Download the package from the [latest release](https://github.com/jp1337/easywall/releases/latest)

```sh
wget https://github.com/jp1337/easywall/releases/download/v0.2.1/easywall_0.2.1-1_amd64.deb
```

- Install the package using Debian Package Manager

```sh
dpkg -i easywall_0.2.1-1_amd64.deb
```

- Optional: Set the username and password for the web interface.

```sh
# optional:
# cd /opt/easywall
python3 easywall/web/passwd.py
```

## Installation by using Git

- Change to the directory where you want to install easywall. We recommend the directories listed below.

```sh
cd /opt
# or
cd /usr/local
```

- Clone the easywall installation of GitHub and change to the newly created directory.

```sh
git clone https://github.com/jp1337/easywall.git
cd easywall
```

- To install the easywall core, run the installation script for the core.

```sh
# optional:
# cd /opt/easywall
bash scripts/install-core.sh
```

- Optional: To install the web interface, run the installation script for the web interface.

```sh
# optional:
# cd /opt/easywall
bash scripts/install-web.sh
```

- Optional: Set the username and password for the web interface.

```sh
# optional:
# cd /opt/easywall
python3 easywall/web/passwd.py
```

## Manual installation

- Change to the directory where you want to install easywall. We recommend the directories listed below.

```sh
cd /opt
# or
cd /usr/local
```

- Download the latest version of easywall from GitHub

```sh
wget https://github.com/jp1337/easywall/archive/master.zip
```

- Unzip the file and rename the folder. Then change to the newly created directory.

```sh
unzip master.zip
mv easywall-master easywall
cd easywall
```

- To install the easywall core, run the installation script for the core.

```sh
# optional:
# cd /opt/easywall
bash scripts/install-core.sh
```

- Optional: To install the web interface, run the installation script for the web interface.

```sh
# optional:
# cd /opt/easywall
bash scripts/install-web.sh
```

- Optional: Set the username and password for the web interface.

```sh
# optional:
# cd /opt/easywall
python3 easywall/web/passwd.py
```
