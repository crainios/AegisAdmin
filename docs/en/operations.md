# Operations guide

[Version française](../operations.md)

## Key locations

| Item | Location or service |
|---|---|
| Web interface | `aegisadmin-web.service` |
| Privileged backend | `aegisadmin-system.service` |
| SQLite database | `/var/lib/aegisadmin/database/aegisadmin.sqlite` |
| Database backups | `/var/lib/aegisadmin/database/backups` |
| Configuration snapshots | `/var/lib/aegisadmin/configuration/snapshots` |
| Configuration | `/etc/aegisadmin-system` |
| Private socket | `/run/aegisadmin-system/backend.sock` |

## Initial setup

```bash
sudo apt install aegisadmin
sudo aegisadmin initialize
sudo aegisadmin setup-status
```

The login identifier is always `root`. Initialization collects the default
language, first name, last name, email address and a 12–72 character password.
If local MySQL/MariaDB monitoring needs separate credentials, run:

```bash
sudo aegisadmin mysql-setup
```

The SQL administrator password is never retained. A random monitoring secret
is stored root-only in `/etc/aegisadmin-system/mysql-client.cnf`.

## Health checks

```bash
sudo systemctl status aegisadmin-system.service aegisadmin-web.service --no-pager -l
sudo aegisadmin version
```

Use `curl -k https://127.0.0.1:9080/readyz` with Apache, or
`curl -k https://127.0.0.1:8443/readyz` without it. The response should report
`status`, `backend` and `database` as `ok`.

## Updates and persistence

Updates run in an independent `aegisadmin-updater@.service` instance. Progress
is persisted under `/var/lib/aegisadmin/updates`, so the terminal modal and the
authenticated session survive AegisAdmin service restarts. When
`/var/run/reboot-required` exists, root may confirm an immediate or delayed
reboot.

## Backup and recovery

Use **Settings > Database backup** for a consistent SQLite copy. Restoration
validates integrity and migrations and creates a safety copy. For encrypted
SMTP credentials to remain usable on another server, separately protect and
restore `/var/lib/aegisadmin/secrets/settings.key` as mode `0600`, owner
`aegisadmin`.

## Diagnostics

```bash
sudo journalctl -u aegisadmin-system.service -n 100 --no-pager -l
sudo journalctl -u aegisadmin-web.service -n 100 --no-pager -l
sudo ss -ltnp | grep -E ':8443|:9080'
```

If Apache is installed, also inspect `apache2.service` and run
`sudo apachectl configtest`. Never start the AegisAdmin web server as root.

Package removal preserves data; purge removes packaged configuration and the
local TLS pair but deliberately leaves the database, backups and snapshots.
