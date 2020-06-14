# Complete deletion of easywall

If you have problems with your easywall installation, you may want to delete easywall completely. You can use these instructions to carry out the solution.

## Perform deletion

1. Installed packages:
   Consider unstalling the following packages:
   `python3 python3-watchdog python3-flask uwsgi uwsgi-plugin-python3 wget unzip`
   You need to be very careful when removing packages! Maybe there is other software using these packages.

2. Remove systemd processes
   Delete these files:

- `/lib/systemd/system/easywall.service`
- `/lib/systemd/system/easywall-web.service`

3. Reload systemd daemons
   `systemctl daemon-reload`

4. Remove easywall user
   `deluser easywall`

5. Remove the installation folder completely
   `rm -r /usr/local/easywall`
