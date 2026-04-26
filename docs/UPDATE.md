# How to update easywall

Always keep easywall up to date to eliminate errors and security gaps as quickly as possible.

You can see the current software status in the web interface.

There you will be notified with an exclamation mark when a new version is available.

To update easywall, follow the simple steps below. Please read the documentation carefully to avoid errors.

## Update with Git

- Change to the directory where you have installed easywall.

```sh
# for instance:
cd /opt/easywall
```

- Download the latest version of GitHub by running the Git command

```sh
git pull
```

- Restart the services to apply the changes.

```sh
systemctl restart easywall
systemctl restart easywall-web
```

## Update through our script

**Please note** that after an update through our script, you can no longer update your installation via Git, as the Git folder will be deleted.

Our update script creates an automatic backup of your previous installation. Nevertheless we recommend to backup the operating system regularly to avoid problems.

If you have installed easywall manually or still want to update through our script, please follow these steps:

- Change to the directory where you have installed easywall.

```sh
# for instance:
cd /opt/easywall
```

- Run our automatic update script and carefully check the output for errors.

```sh
bash scripts/update.sh
```
