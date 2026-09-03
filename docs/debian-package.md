# Paquet Debian

Le paquet contient les cinq binaires Go, les migrations SQL, les profils
système et les ressources graphiques nécessaires.

```bash
bash packaging/build-deb.sh --arch amd64 --output dist
sudo apt install ./dist/aegisadmin_VERSION_amd64.deb
sudo aegisadmin initialize
```

Le serveur web Go écoute selon `/etc/aegisadmin-system/web-server`. Sans
Apache, il fournit directement HTTPS sur le port `8443`. Lorsqu’Apache est
déjà installé, AegisAdmin conserve une écoute Go locale sur `127.0.0.1:9080`
et configure Apache comme proxy inverse. Apache n’est jamais installé
implicitement par le paquet AegisAdmin.

Une mise à niveau conserve les fichiers déclarés comme configuration Debian,
la base SQLite, ses sauvegardes, les snapshots et la paire TLS locale. Si le
certificat ou la clé TLS manque alors que l’autre fichier existe encore, la
configuration du paquet s’arrête sans remplacer le fichier restant.

Vérifications :

```bash
sudo systemctl status aegisadmin-system.service aegisadmin-web.service --no-pager
curl -k https://127.0.0.1:8443/readyz
sudo aegisadmin version
```
