# Debian lifecycle tests

[Version française](../debian-lifecycle-tests.md)

Run these checks only on a disposable VM or a server with a verified snapshot.

1. Build and verify with `bash packaging/build-deb.sh --arch amd64 --output dist`.
2. Install, then run `sudo aegisadmin initialize`.
3. Verify `aegisadmin-system.service`, `aegisadmin-web.service` and the proper
   ready endpoint (`9080` with Apache, `8443` without it).
4. Create a user, setting and configuration snapshot.
5. Reinstall the same package and verify that accounts, settings, snapshots and
   TLS fingerprints are preserved.
6. Upgrade from the previous candidate and verify the same data and the visible
   version.
7. Remove and reinstall; persistent data must remain while sessions must not.
8. Purge only after a VM snapshot; configuration and TLS files must disappear,
   while database backups and configuration snapshots remain deliberately.

Never treat a changing SQLite file hash as proof of data loss: migrations and
normal writes legitimately change it. Verify logical records instead.
