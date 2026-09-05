# AegisAdmin backend

[Version française](README.fr.md)

The active backend is entirely written in Go under `backend/go`.

- `config`: system profiles installed under `/etc/aegisadmin-system`;
- `libexec`: restricted Python runners used by Cron and Certbot;
- `go`: privileged daemon, web interface, administration tools and installer.

Initial installation must use the Debian package. An already packaged test
installation may then be checked and upgraded from a working tree:

```bash
bash backend/go/install.sh check
sudo bash backend/go/install.sh install
sudo bash backend/go/install.sh verify
```

The source installer updates the same binaries and
`aegisadmin-system.service` as the package. It refuses initial installation
when the packaged web service is absent.
