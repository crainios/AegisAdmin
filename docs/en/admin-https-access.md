# HTTPS access and optional Apache proxy

[Version française](../admin-https-access.md)

AegisAdmin has two package-selected modes:

- without Apache, Go serves HTTPS directly on port `8443`;
- with Apache, Go listens on `https://127.0.0.1:9080` and Apache publishes the
  dedicated `8443` access.

Initial access is therefore `https://SERVER_IP:8443` in both cases. Port 9080
must never be exposed to the Internet. The initial local certificate is not
publicly trusted; verify its fingerprint before accepting it:

```bash
sudo openssl x509 \
  -in /etc/aegisadmin-system/tls/admin-local.crt \
  -noout -sha256 -fingerprint
```

Root can enable or disable dedicated HTTPS and change its port, listen
addresses and allowed network under **Settings > Dedicated HTTPS access**.
Firewall rules are deliberately managed separately. Verify another working
access path before disabling the dedicated endpoint.

When Apache is installed, a public domain can use a separate port 443
VirtualHost that proxies to `https://127.0.0.1:9080`. The dedicated 8443 access
may remain available as a recovery path.
