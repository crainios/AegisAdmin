# Private download dashboard

[Version française](../download-statistics.md)

The standalone dashboard aggregates successful APT package requests from
`aegisadmin-packages-access.log*` and public GitHub Release asset counters. It
does not copy IP addresses into generated statistics. Counts represent package
transfers, not guaranteed distinct installations.

Build the static Linux binary on the development machine:

```bash
bash packaging/download-stats/build.sh
```

On the package server, create Basic Authentication credentials outside the
DocumentRoot:

```bash
sudo apt install apache2-utils
sudo htpasswd -c /etc/apache2/aegisadmin-download-stats.htpasswd USERNAME
sudo chown root:www-data /etc/apache2/aegisadmin-download-stats.htpasswd
sudo chmod 0640 /etc/apache2/aegisadmin-download-stats.htpasswd
```

Transfer the generated binary and `packaging/download-stats/`, then run:

```bash
sudo bash install.sh /path/to/aegisadmin-download-stats-amd64
```

The installer adds a hardened oneshot service, an hourly timer and an
authenticated Apache alias at:

```text
https://packages.aegisadmin.fr/private-downloads/
```

GitHub counts remain zero until `.deb` files are attached to GitHub Releases.
Anonymous access is sufficient for the public repository; an optional
`GITHUB_TOKEN` may be supplied through a protected systemd environment if API
limits later require it. Configure source-log retention separately with
`logrotate`.
