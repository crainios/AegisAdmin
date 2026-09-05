# AegisAdmin Go deployment

[Version française](DEPLOYMENT.fr.md)

AegisAdmin uses a Go daemon and Go web server. Package updates run through an
independent `aegisadmin-updater@.service` instance and persist progress under
`/var/lib/aegisadmin/updates`.

## Validate an installed test working tree

```bash
cd /path/to/AegisAdmin
bash backend/go/install.sh check
sudo bash backend/go/install.sh install
sudo bash backend/go/install.sh verify
```

Initial deployment must use the Debian package. The source installer is only
for an existing packaged installation.

## Verify

```bash
/usr/bin/aegisadmin-system-go --version
/usr/libexec/aegisadmin/aegisadmin-admin --version
/usr/libexec/aegisadmin/aegisadmin-web --version
systemctl status aegisadmin-system.service aegisadmin-web.service --no-pager
```

The database is `/var/lib/aegisadmin/database/aegisadmin.sqlite`, configuration
is under `/etc/aegisadmin-system`, and restricted Cron and Certbot runners are
installed under `/usr/libexec/aegisadmin`.

## Root account

```bash
sudo aegisadmin initialize
sudo /usr/libexec/aegisadmin/aegisadmin-admin reset-root-password
sudo /usr/libexec/aegisadmin/aegisadmin-admin disable-root-two-factor
sudo /usr/libexec/aegisadmin/aegisadmin-admin migrate
```

Without Apache, Go serves HTTPS on `8443`. With Apache, it listens locally on
`127.0.0.1:9080` and Apache acts only as a reverse proxy.
