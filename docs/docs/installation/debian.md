# Debian / Ubuntu Installation

## From the GitHub Release

1. **Download the package** from the [latest release](https://github.com/jp1337/easywall/releases/latest):

    ```bash
    wget https://github.com/jp1337/easywall/releases/latest/download/easywall_amd64.deb
    ```

2. **Install:**

    ```bash
    sudo dpkg -i easywall_amd64.deb
    sudo apt-get install -f    # resolve any missing dependencies
    ```

3. **Open the web interface** in your browser:

    ```
    https://<server-ip>:12227
    ```

    Your browser will warn about the self-signed certificate — this is expected.
    Accept the security exception to proceed.

4. **Complete the first-run wizard**: set your username and password.

## After Installation

### Service Management

```bash
# Check service status
systemctl status easywall-core easywall-web

# View logs
journalctl -u easywall-core -f
journalctl -u easywall-web  -f

# Restart services
systemctl restart easywall-core easywall-web
```

### Configuration Files

| File | Purpose |
|---|---|
| `/etc/easywall/easywall.toml` | Core daemon config (firewall options) |
| `/etc/easywall/web.toml` | Web server config (auth, TLS, language) |
| `/var/lib/easywall/rules.json` | Firewall rules (current/staged/backup) |
| `/var/log/easywall/audit.log` | Audit log |

### Custom TLS Certificate

To use a Let's Encrypt or other certificate, edit `/etc/easywall/web.toml`:

```toml
[tls]
cert = "/etc/letsencrypt/live/example.com/fullchain.pem"
key  = "/etc/letsencrypt/live/example.com/privkey.pem"
```

Then restart the web service:

```bash
systemctl restart easywall-web
```

## Uninstallation

```bash
sudo apt remove easywall
```

This stops and disables the services but preserves `/etc/easywall` and
`/var/lib/easywall`. To also remove data:

```bash
sudo apt purge easywall
sudo rm -rf /etc/easywall /var/lib/easywall /var/log/easywall
```
