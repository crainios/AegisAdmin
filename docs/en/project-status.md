# Project status

[Version française](../PROJECT_STATUS.md)

Current candidate: **0.2.86**.

AegisAdmin 0.2.x is a feature-complete release candidate. Its main interface,
Go architecture, Debian package and signed APT repository are operational.
Functional validation continues on test servers before the first stable
release.

Available areas include system monitoring, processes, storage, services,
networking, logs, Apache, PHP-FPM, MySQL/MariaDB, Tor, Fail2ban, UFW, Cron,
Certbot, APT and firmware updates, Composer monitoring, users, per-module
permissions, TOTP, access auditing, settings, SQLite backups and server
configuration snapshots.

The current public package targets Debian or Ubuntu with systemd, APT and
`amd64`. Apache is optional. Without it, Go serves HTTPS on port `8443`; with
it, Apache proxies to the local Go listener on `127.0.0.1:9080`.

Use a recoverable test machine and retain independent SSH access while the
candidate series is being validated.
