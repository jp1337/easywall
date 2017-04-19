# How to Install EasyWall

You just need three simple steps for installing EasyWall on your system.

---

#### 1. Prepare your System

```sh
apt-get update
apt-get install bash apache2 php5 git
```

You can replace **apache2 and php5** if you want to use other pachages like **nginx** or self compiled php version. You can also use **PHP CGI** to use EasyWall.


#### 2. Clone GitHub Repository with git client

```sh
cd /usr/local
git clone https://github.com/KingJP/EasyWall.git
cd EasyWall
```

You can **clone** the repository to get the **latest version** of EasyWall. You also can **download** the repository from GitHub and **unzip** it on your system.
We suggest to use the directories **/usr/local** or **/opt** for EasyWall Installation.


#### 3. Execute self-installer

```sh
bash install.sh
```

We suggest to use the installer for proper installation. **You have to execute the installer as root user!** For manual installation execute the steps in the installer file on your own.
