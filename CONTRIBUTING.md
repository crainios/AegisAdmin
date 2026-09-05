# Contributing to AegisAdmin

[Version française](CONTRIBUTING.fr.md)

The 0.2.x series is in stabilization. Reproducible bug fixes, security
improvements, tests and documentation clarifications have priority.

## Before starting

- check whether an issue already describes the problem;
- never include real server data, private addresses, keys or secrets;
- report vulnerabilities only through the private process in
  [SECURITY.md](SECURITY.md);
- test every system operation on an isolated, recoverable machine.

## Repository layout

- `backend/go/`: Go web server, privileged backend and tests;
- `backend/config/`: installed system profiles;
- `backend/libexec/`: restricted Cron and Certbot runners;
- `database/migrations/`: SQLite migrations;
- `packaging/`: Debian package and APT repository tooling;
- `docs/`: French documentation and the English index;
- `docs/en/`: English documentation;
- `public/assets/`: shared graphical assets.

## Local checks

From `backend/go`:

```bash
gofmt -w .
go test ./...
go vet ./...
```

Before submitting packaging changes, from the repository root:

```bash
bash -n backend/go/install.sh
bash packaging/build-deb.sh
bash packaging/check-deb.sh dist/aegisadmin_$(cat VERSION)_amd64.deb
```

## Contribution principles

- preserve the privilege boundary between the web server and backend;
- strictly validate all data before a system operation;
- avoid shell execution when an argument-based invocation is possible;
- enforce View, Actions and Modify permissions server-side;
- add or update tests for every behavior change;
- keep user-interface text available in both French and English;
- update both documentation languages and `CHANGELOG.md` when required.

Contributions are distributed under the project's GNU AGPL v3.0 license.
