# Application settings

[Version française](../settings.md)

The root-only `/setting` page manages the default log source, Certbot email,
default French or English interface language, dedicated HTTPS access, SMTP
configuration and SQLite backup or restoration.

SMTP credentials are encrypted in SQLite with AES-GCM. The separate key is
stored at `/var/lib/aegisadmin/secrets/settings.key`; the visible password mask
never contains the secret. SMTP fields are ready for future notifications, but
AegisAdmin does not send email yet.

Restoration validates the SQLite header, integrity, migrations and unique active
root account before replacement, and creates a safety copy first. The ten most
recent safety copies are kept under `/var/lib/aegisadmin/database/backups`.
