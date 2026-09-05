# Validation du cycle de vie Debian

Ces essais doivent être réalisés sur une VM dédiée ou sur un serveur disposant
d’un snapshot restaurable. Les commandes de suppression ne doivent pas être
exécutées sur le serveur de test principal sans sauvegarde vérifiée.

## 1. Construire et contrôler le paquet

```bash
bash packaging/build-deb.sh --arch amd64 --output dist
bash packaging/check-deb.sh dist/aegisadmin_VERSION_amd64.deb
```

Le second contrôle est aussi exécuté automatiquement par le constructeur. Il
vérifie notamment la version, les scripts Debian, les droits, les fichiers de
configuration et l’absence de données persistantes ou de clés TLS embarquées.

## 2. Installation neuve

Sur une VM Debian sans AegisAdmin :

```bash
sudo apt install ./aegisadmin_VERSION_amd64.deb
sudo aegisadmin initialize
sudo systemctl status aegisadmin-system.service aegisadmin-web.service --no-pager
```

Avec Apache, contrôler `apache2.service` et
`https://127.0.0.1:9080/readyz`. Sans Apache, contrôler directement
`https://127.0.0.1:8443/readyz`.

Créer ensuite un utilisateur de test, un paramètre et un snapshot, puis noter
les empreintes de la base et de la paire TLS :

```bash
sudo sha256sum /var/lib/aegisadmin/database/aegisadmin.sqlite
sudo sha256sum /etc/aegisadmin-system/tls/admin-local.crt
sudo sha256sum /etc/aegisadmin-system/tls/admin-local.key
```

## 3. Réinstallation

```bash
sudo apt install --reinstall ./aegisadmin_VERSION_amd64.deb
```

Contrôler la sonde sur `9080` avec Apache ou `8443` sans Apache.

Contrôler que le compte, les paramètres et les snapshots sont toujours présents
et que les empreintes TLS n’ont pas changé. L’empreinte SQLite peut évoluer du
fait des migrations et ne doit pas être utilisée seule comme preuve de
conservation logique des données.

## 4. Mise à niveau

Installer d’abord le paquet de la version précédente, créer les mêmes données
témoins, puis installer le nouveau paquet :

```bash
sudo apt install ./aegisadmin_NOUVELLE_VERSION_amd64.deb
sudo systemctl status aegisadmin-system.service aegisadmin-web.service --no-pager
```

Utiliser la sonde `9080` avec Apache ou `8443` sans Apache.

Vérifier la version dans l’interface et avec `sudo aegisadmin version`, puis
contrôler les données témoins, les fichiers de configuration, les snapshots et
les empreintes TLS.

## 5. Suppression et réinstallation

```bash
sudo apt remove aegisadmin
sudo test -f /var/lib/aegisadmin/database/aegisadmin.sqlite
sudo apt install ./aegisadmin_VERSION_amd64.deb
```

La suppression arrête les services et désactive le VirtualHost dédié lorsqu’il
existe, mais conserve volontairement la base, les snapshots, les sauvegardes et
la paire TLS.

## 6. Purge

Après avoir créé un snapshot de la VM :

```bash
sudo apt purge aegisadmin
sudo test -f /var/lib/aegisadmin/database/aegisadmin.sqlite
sudo test ! -e /etc/aegisadmin-system/tls/admin-local.crt
sudo test ! -e /etc/aegisadmin-system/tls/admin-local.key
```

La purge retire la paire TLS locale et les configurations gérées par Debian.
La base, ses sauvegardes et les snapshots restent volontairement conservés afin
d’éviter toute destruction implicite de données administratives.
