# Debian package

[Version française](../debian-package.md)

The package contains the Go executables, SQL migrations, system profiles and
web assets. Build and install a local candidate with:

```bash
bash packaging/build-deb.sh --arch amd64 --output dist
sudo apt install ./dist/aegisadmin_VERSION_amd64.deb
sudo aegisadmin initialize
```

Without Apache, the web service provides HTTPS on `8443`. If Apache is already
installed, the package keeps Go on `127.0.0.1:9080` and configures a reverse
proxy. It never installs Apache implicitly.

Upgrades preserve conffiles, SQLite data, backups, snapshots and the local TLS
pair. If only one TLS file remains, package configuration stops rather than
silently replacing its counterpart.
