# Server configuration snapshots

[Version française](../server-configuration.md)

The root-only Configuration module collects a read-only server inventory and
creates SHA-256-protected snapshots. Sections cover the operating system,
packages, services, accounts, network, listening ports, firewall, Fail2ban,
PHP, Apache, certificates, MySQL/MariaDB, Cron and Tor.

Secrets such as `/etc/shadow`, passwords, private keys, TOTP secrets and
cookies are never collected. Snapshots are stored under
`/var/lib/aegisadmin/configuration/snapshots`, can be named, exported, imported
from another server, deleted after explicit confirmation and compared by
section. Import validates schema, identifier, size and fingerprint.

Comparison is currently read-only. Future stages may add semantic migration
plans and proposed corrections.
