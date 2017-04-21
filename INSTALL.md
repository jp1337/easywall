# How to Install EasyWall

You just need three simple steps for installing EasyWall on your system.

## Index

1. [Check Requirements](#require)
2. [Installing on Debian / Ubuntu / Linux Mint](#debian)
3. [Download and Install EasyWall](#easywall)

## <a name="require"></a> 1. Check Requirements

- ssh (root access) on the server
- Apache Web server (at least version 2)
- PHP version >= 5 with

## <a name="debian"></a> 2. Installing on Debian / Ubuntu / Linux Mint

```sh
sudo apt-get update
sudo apt-get install bash git apache2 apache2-utils libapache2-mod-php5 php5
```

You can replace **apache2 and php5** if you want to use other pachages like **nginx** or self compiled php version. You can also use **PHP CGI** to use EasyWall.

## <a name="easywall"></a> 3. Download and Install EasyWall

```sh
cd /usr/local
git clone https://github.com/KingJP/EasyWall.git
cd EasyWall
```

You can **clone** the repository to get the **latest version** of EasyWall. You also can **download** the repository from GitHub and **unzip** it on your system.
We suggest to use the directories **/usr/local** or **/opt** for EasyWall Installation.

```sh
sudo chmod +x install.sh
sudo bash install.sh
```

We suggest to use the installer for proper installation. **You have to execute the installer as root user!** For manual installation execute the steps in the installer file on your own.
