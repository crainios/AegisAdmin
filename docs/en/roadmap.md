# Roadmap

[Version française](../ROADMAP.md)

## 0.2.x stabilization

1. Continue clean-install and upgrade testing on Debian and Ubuntu.
2. Validate normal behavior when optional software is absent.
3. Fix issues found through real server use.
4. Extend automated Go, package and lifecycle checks.
5. Validate and document every candidate release.
6. Add SMTP testing and configurable notifications.

## First stable release

The first stable release will follow validation of installation,
authentication, read-only monitoring, system actions, backups, restoration and
self-updates. Initial support will target `amd64` Debian/Ubuntu systems with
systemd and APT; Apache remains optional.

Later work includes official `arm64` packages, multi-server coordination,
semantic migration plans from configuration snapshots and carefully defined
extension interfaces.
