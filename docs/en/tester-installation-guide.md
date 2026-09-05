# Download, verify and install AegisAdmin

[Version française](../tester-installation-guide.md)

Use a test machine with a restorable backup, independent SSH access and a sudo
account. The current public candidate targets `amd64` Debian or Ubuntu with
systemd and APT. Apache is optional.

## Configure the signed repository

Download the key without installing it and verify its fingerprint:

```bash
wget --https-only -O /tmp/aegisadmin-archive-keyring.gpg \
  https://packages.aegisadmin.fr/aegisadmin-archive-keyring.gpg
gpg --show-keys --with-fingerprint /tmp/aegisadmin-archive-keyring.gpg
```

Expected primary fingerprint:

```text
D01E 7409 36E2 CB6E 7BA8  76F9 C103 5511 5D7F 97DB
```

Stop if it differs. Install the key and repository declaration published at
`https://packages.aegisadmin.fr/aegisadmin.sources`, then run:

```bash
sudo apt update
apt policy aegisadmin
sudo apt install aegisadmin
sudo aegisadmin initialize
```

Initialization selects the default language and creates the AegisAdmin root
account. Its login is always `root`. If required, complete SQL monitoring with
`sudo aegisadmin mysql-setup`; the SQL administrator password is not stored.

## Verify and connect

```bash
sudo systemctl status aegisadmin-system.service aegisadmin-web.service --no-pager -l
sudo aegisadmin version
```

Open `https://SERVER_IP:8443/`. Verify the local certificate fingerprint before
accepting the browser warning. The firewall is not opened automatically.

With Apache, Go remains local on `127.0.0.1:9080`; without Apache, Go serves
8443 directly. Test the matching `/readyz` endpoint. Start with read-only pages
and create a database backup before testing modifications.

For diagnostics, inspect both AegisAdmin service journals and listening ports.
Inspect Apache only when installed. Never run `aegisadmin-web` as root and never
send secrets or personal data in a test report.
