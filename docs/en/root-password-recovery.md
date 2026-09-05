# Root account recovery

[Version française](../root-password-recovery.md)

Use local console or independent SSH access. Reset the AegisAdmin root password
with:

```bash
sudo /usr/libexec/aegisadmin/aegisadmin-admin reset-root-password
sudo systemctl restart aegisadmin-web.service
```

If the TOTP device is lost, disable root two-factor authentication with:

```bash
sudo /usr/libexec/aegisadmin/aegisadmin-admin disable-root-two-factor
sudo systemctl restart aegisadmin-web.service
```

Both operations invalidate existing root sessions. They do not change the
operating-system root password.
