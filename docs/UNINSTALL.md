# How to uninstall easywall

If you are dissatisfied with easywall or want to replace the software with another, you can uninstall easywall.

To uninstall, the following simple steps must be carried out.

Please read the instructions carefully to avoid errors.

## Performing the uninstallation

- Change to the directory where you have installed easywall.

```sh
# for instance:
cd /opt/easywall
```

- Run our automatic uninstallation script and carefully check the output for errors.

```sh
bash scripts/update.sh
```

- Delete the installation folder of easywall with the following command.

```sh
# for instance:
rm -r /opt/easywall
```

- The following operating system packages may have been installed by easywall. The packages may be required by other software. Please check the packages carefully before you delete them.

**Packages:**

```text
python3
python3-pip
uwsgi
uwsgi-plugin-python3
wget
unzip
openssl
```
