# How to Install EasyWall

You just need three simple steps for installing EasyWall on your system.

## Index

1. [Check Requirements](#require)
2. [Download and Install EasyWall](#easywall)

## <a name="require"></a> 1. Check Requirements

- ssh (root access) on the server
- Currently we only support Debian based Servers

## <a name="easywall"></a> 2. Download and Install EasyWall

```sh
cd /usr/local
git clone https://github.com/jp1337/easywall.git
cd easywall
```

You can **clone** the repository to get the **latest version** of EasyWall. You also can **download** the repository from GitHub and **unzip** it on your system.
We suggest to use the directories **/usr/local** or **/opt** for EasyWall Installation.

```sh
sudo chmod +x install.sh
sudo -H bash install.sh
```

We suggest to use the installer for proper installation. **You have to execute the installer as root user!** For manual installation execute the steps in the installer file on your own.
