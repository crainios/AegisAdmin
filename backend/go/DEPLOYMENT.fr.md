# Déploiement AegisAdmin Go

AegisAdmin utilise exclusivement un daemon et un serveur web Go. Le frontend
AegisAdmin utilise exclusivement ses composants Go pour l'interface web et le
backend système.

Les installations de paquets lancées depuis l'interface sont exécutées par
`aegisadmin-updater@.service`. Cette unité indépendante conserve son état sous
`/var/lib/aegisadmin/updates` et n'est pas arrêtée lorsque le paquet redémarre
le backend ou le serveur web.

## Validation et installation

```bash
cd /var/www/Aegisadmin
bash backend/go/install.sh check
sudo bash backend/go/install.sh install
```

L’installation compile et déploie `aegisadmin-daemon`, `aegisadmin-system-go`,
`aegisadmin-web` et `aegisadmin-admin`, applique les migrations SQLite, installe
les profils système et redémarre les services déjà présents.

## Vérifications

```bash
/usr/bin/aegisadmin-system-go --version
/usr/libexec/aegisadmin/aegisadmin-admin --version
/usr/libexec/aegisadmin/aegisadmin-web --version
systemctl status aegisadmin-system.service aegisadmin-web.service --no-pager
```

La base SQLite active est `/var/lib/aegisadmin/database/aegisadmin.sqlite`.
Les profils sont placés sous `/etc/aegisadmin-system`. Les exécuteurs Python de
Cron et Certbot sont installés sous `/usr/libexec/aegisadmin`.

## Compte root

```bash
sudo aegisadmin initialize
sudo /usr/libexec/aegisadmin/aegisadmin-admin reset-root-password
sudo /usr/libexec/aegisadmin/aegisadmin-admin disable-root-two-factor
sudo /usr/libexec/aegisadmin/aegisadmin-admin migrate
```

Le paquet choisit l’écoute HTTPS selon la présence d’Apache : directement sur
`:8443` sans Apache, ou sur `127.0.0.1:9080` derrière son proxy inverse. Pour
une publication par nom de domaine avec Apache, celui-ci transmet vers cette
adresse locale ; il n’exécute aucun code applicatif AegisAdmin.
