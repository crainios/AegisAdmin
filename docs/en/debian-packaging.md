# Debian package layout

[Version française](../architecture/debian-packaging.md)

| Purpose | Path |
|---|---|
| Administration command | `/usr/bin/aegisadmin` |
| System CLI | `/usr/bin/aegisadmin-system-go` |
| Privileged daemon | `/usr/libexec/aegisadmin/aegisadmin-daemon` |
| Web server | `/usr/libexec/aegisadmin/aegisadmin-web` |
| SQLite/root administration | `/usr/libexec/aegisadmin/aegisadmin-admin` |
| Update runner | `/usr/libexec/aegisadmin/aegisadmin-updater` |
| Migrations | `/usr/share/aegisadmin/migrations` |
| Configuration | `/etc/aegisadmin-system` |
| Persistent data | `/var/lib/aegisadmin` |
| Private socket | `/run/aegisadmin-system/backend.sock` |

The web server runs as `aegisadmin`. The privileged daemon remains separate;
membership of `aegisadmin-web` controls access to the Unix socket.
