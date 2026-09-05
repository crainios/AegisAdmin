# AegisAdmin

[Documentation française](README.fr.md)

AegisAdmin is a lightweight web administration and monitoring interface for
Debian and Ubuntu servers. The 0.2.x series is a release candidate intended for
testing on a recoverable machine with an independent SSH access path.

## Main features

- dashboard, system resources, processes, storage, network and logs;
- systemd services, Apache VirtualHosts, PHP-FPM, MySQL/MariaDB and Tor;
- Fail2ban, UFW firewall and TLS certificates with Certbot;
- Cron tasks and execution results;
- APT, firmware and hosted-site Composer dependency updates;
- users, per-module permissions, access log and TOTP two-factor authentication;
- settings, SQLite backups and server-configuration snapshots;
- French and English interface with four persistent themes.

## Architecture and security

The application separates two Go components:

- `aegisadmin-web`, running as the unprivileged `aegisadmin` account;
- `aegisadmin-daemon`, the privileged system backend, reachable only through a
  protected Unix socket.

System operations are explicitly registered and validated. Permissions are
enforced server-side at View, Actions or Modify level. AegisAdmin also provides
CSRF protection, secure persistent sessions, TOTP and an access audit log.

Without Apache, the Go web server provides HTTPS directly on port `8443`. If
Apache is already installed, the package configures it as a reverse proxy to
`https://127.0.0.1:9080`. AegisAdmin never installs Apache implicitly.

See the [architecture](docs/en/architecture.md) and
[security policy](SECURITY.md).

## Supported systems

The current public release targets Debian or Ubuntu with systemd, APT and the
`amd64` architecture. An `arm64` package can be built from the source but still
requires validation before official publication.

## Install the release candidate

Packages are distributed through the official signed APT repository:

- website: <https://aegisadmin.fr/>;
- package repository: <https://packages.aegisadmin.fr/>;
- [installation and testing guide](docs/en/tester-installation-guide.md).

Never disable APT signature verification. After package installation, create
the root account and select the default interface language:

```bash
sudo aegisadmin initialize
```

## Develop and verify

Go 1.22 or a compatible version is required on the development machine only.

```bash
cd backend/go
go test ./...
go vet ./...
```

Build and verify the Debian package from the repository root:

```bash
bash packaging/build-deb.sh
bash packaging/check-deb.sh dist/aegisadmin_$(cat VERSION)_amd64.deb
```

Deployment scripts modify the operating system and must only be used on an
appropriate test machine.

## Documentation

The [English documentation index](docs/README.md) covers installation,
operations, architecture, permissions, recovery and packaging. The
[French documentation index](docs/README.fr.md) remains available alongside it.

Current release status is tracked in the
[project status](docs/en/project-status.md) and planned work in the
[roadmap](docs/en/roadmap.md).

## Contributing and reporting security issues

Read [CONTRIBUTING.md](CONTRIBUTING.md) before submitting a change. Do not open
a public issue for a vulnerability; follow [SECURITY.md](SECURITY.md).

## License

AegisAdmin is licensed under the
[GNU Affero General Public License v3.0](LICENSE).
