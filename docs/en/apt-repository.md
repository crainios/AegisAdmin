# Signed APT repository

[Version française](../apt-repository.md)

The official repository is `https://packages.aegisadmin.fr`, suite `stable`,
component `main`, currently published for `amd64`. Clients must use a dedicated
keyring and a deb822 `.sources` file with `Signed-By`; never use `apt-key`,
`trusted=yes` or disabled signature checks.

Release maintainers build candidates with `packaging/build-release.sh`, create
repository metadata with `packaging/build-apt-repository.sh`, sign the
`Release` metadata, then publish atomically with `packaging/publish-release.sh`.
The public link must only switch after package validation, metadata checks,
signature verification and upload success.

The current official primary-key fingerprint is:

```text
D01E 7409 36E2 CB6E 7BA8  76F9 C103 5511 5D7F 97DB
```

Confirm current signing information through an independent official channel
before publishing or trusting a new key.
