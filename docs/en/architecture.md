# AegisAdmin architecture

[Version française](../ARCHITECTURE.md)

```text
HTTPS browser
    │
    ▼
aegisadmin-web (unprivileged account)
    │ protected Unix socket
    ▼
aegisadmin-daemon (validated privileged operations)
    │
    ▼
Linux, systemd and administration tools
```

The Go web server manages authentication, sessions, the interface and SQLite.
The privileged backend exposes only explicitly registered domains and commands.
Persistent data is stored under `/var/lib/aegisadmin`, configuration under
`/etc/aegisadmin-system`, and the private socket at
`/run/aegisadmin-system/backend.sock`.

HTML, CSS and JavaScript are embedded in the web binary. The remaining Python
runners are restricted to Cron and Certbot; shell scripts are limited to
installation and packaging.
