# Two-factor authentication

[Version française](../two-factor-authentication.md)

AegisAdmin supports TOTP codes compatible with Google Authenticator. Root may
require two-factor authentication for a user. When required but not yet
configured, the user must scan the QR code and confirm a current code before
accessing the dashboard.

When optional, users can enable or disable TOTP under **My account**. Login first
validates the password, then displays a separate code challenge for accounts
with active TOTP. Secrets are stored in SQLite and must never appear in logs,
snapshots or process arguments.

If root loses the authenticator, use the local recovery command documented in
[Root account recovery](root-password-recovery.md). Recovery invalidates active
root sessions.
