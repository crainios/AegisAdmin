# AegisAdmin Go

[Version française](README.fr.md)

This module contains five Go executables:

- `aegisadmin-daemon`: validated privileged operations over a Unix socket;
- `aegisadmin-system-go`: system administration CLI;
- `aegisadmin-web`: unprivileged HTTPS web server;
- `aegisadmin-admin`: SQLite migrations and root account recovery;
- `aegisadmin-updater`: independent, persistent system update runner.

Validate a working tree with:

```bash
bash backend/go/install.sh check
```

The active SQLite database is
`/var/lib/aegisadmin/database/aegisadmin.sqlite`; system profiles remain under
`/etc/aegisadmin-system`. The package serves HTTPS directly on `8443` without
Apache, or uses `127.0.0.1:9080` behind Apache when it is installed.
