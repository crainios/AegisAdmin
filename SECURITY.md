# Security policy

[Version française](SECURITY.fr.md)

## Supported versions

The 0.2.x series is a release candidate. Security fixes are applied to the
latest version published in the official APT repository. Older candidates may
not receive separate fixes.

## Report a vulnerability

Do not open a public issue for a real or suspected vulnerability. Use
**Report a vulnerability** in the GitHub repository Security tab. This creates
a private security advisory visible only to the project maintainers.

Include, when possible:

- the AegisAdmin version and operating system;
- the affected component and permission level;
- the conditions required to reproduce the issue;
- the estimated impact;
- a minimal reproduction procedure;
- any known mitigation.

Never send passwords, private keys, TOTP secrets, session cookies, real
database backups or logs containing personal data. Replace sensitive values
with fictional examples. Do not disclose details publicly before a fix is
available or the maintainers explicitly agree.

## Safe evaluation

AegisAdmin administers sensitive system components. Evaluate the release
candidate first on a virtual machine or test server with a restorable backup
and independent SSH access.

Packages must come from `https://packages.aegisadmin.fr` and be verified using
the official repository key. Never use `trusted=yes`, `apt-key` or an option
that ignores signature failures.
