# Guide d’exploitation AegisAdmin

Ce guide regroupe les opérations courantes de la version 0.2.x installée avec
le paquet Debian. Les commandes de développement depuis les sources sont
décrites séparément dans `backend/go/DEPLOYMENT.md`.

## Repères

| Élément | Emplacement ou service |
|---|---|
| Interface web Go | `aegisadmin-web.service` |
| Backend privilégié | `aegisadmin-system.service` |
| Proxy HTTPS | `apache2.service` |
| Écoute Go locale | `https://127.0.0.1:9080` |
| Accès HTTPS de secours | port `8443` via Apache |
| Base SQLite | `/var/lib/aegisadmin/database/aegisadmin.sqlite` |
| Sauvegardes automatiques | `/var/lib/aegisadmin/database/backups` |
| Snapshots | `/var/lib/aegisadmin/configuration/snapshots` |
| Configuration | `/etc/aegisadmin-system` |
| Certificat local | `/etc/aegisadmin-system/tls/admin-local.crt` |
| Clé locale | `/etc/aegisadmin-system/tls/admin-local.key` |
| Socket privé | `/run/aegisadmin-system/backend.sock` |

## Installation initiale

```bash
sudo apt install ./aegisadmin_VERSION_amd64.deb
sudo aegisadmin initialize
```

L’identifiant du compte administrateur est toujours `root`. Le nom, le prénom
et l’adresse e-mail demandés pendant l’initialisation décrivent ce compte mais
ne remplacent pas son identifiant. Le mot de passe doit contenir entre 12 et 72
caractères.

## Contrôle de fonctionnement

```bash
sudo systemctl status \
    aegisadmin-system.service \
    aegisadmin-web.service \
    apache2.service \
    --no-pager -l

curl --insecure https://127.0.0.1:9080/readyz
sudo aegisadmin version
```

La sonde doit indiquer `status`, `backend` et `database` à `ok`. Le port 9080
doit rester limité à l’interface locale lorsqu’Apache publie l’application.

## Journaux de diagnostic

```bash
sudo journalctl -u aegisadmin-system.service -n 100 --no-pager -l
sudo journalctl -u aegisadmin-web.service -n 100 --no-pager -l
sudo journalctl -u apache2.service -n 100 --no-pager -l
sudo apachectl configtest
```

En cas d’interface indisponible, contrôler dans cet ordre le backend, le socket,
le serveur web, la sonde locale, puis Apache. Le serveur web ne doit jamais être
exécuté en root.

## Mise à niveau

Copier le paquet dans un répertoire accessible à `_apt`, puis l’installer :

```bash
sudo install -m 0644 aegisadmin_VERSION_amd64.deb /tmp/aegisadmin_VERSION_amd64.deb
sudo apt install /tmp/aegisadmin_VERSION_amd64.deb
```

La mise à niveau applique les migrations, conserve la base, les sauvegardes,
les snapshots, la paire TLS et les configurations locales, puis contrôle que
les trois services restent actifs.

## Sauvegarde et restauration

La méthode normale est **Paramètres > Sauvegarde de la base**. Elle produit une
sauvegarde SQLite cohérente sans interrompre le service. La restauration depuis
le même écran contrôle l’intégrité, les migrations et le compte root, puis crée
automatiquement une copie de sécurité avant remplacement.

Pour une copie manuelle hors interface, arrêter brièvement le serveur web :

```bash
sudo systemctl stop aegisadmin-web.service
sudo install -d -m 0700 /var/backups/aegisadmin-manual
sudo cp -a /var/lib/aegisadmin/database/aegisadmin.sqlite \
    /var/backups/aegisadmin-manual/aegisadmin.sqlite
sudo systemctl start aegisadmin-web.service
```

Ne jamais copier uniquement la base pendant une écriture active sans utiliser
la sauvegarde intégrée.

## Récupération du compte root

```bash
sudo /usr/libexec/aegisadmin/aegisadmin-admin reset-root-password
sudo /usr/libexec/aegisadmin/aegisadmin-admin disable-root-two-factor
sudo systemctl restart aegisadmin-web.service
```

La première commande remplace le mot de passe et invalide les sessions. La
seconde retire le secret TOTP et invalide également les sessions existantes.

## HTTPS et certificat local

Le paquet crée une paire TLS locale uniquement si les deux fichiers sont
absents. Si un seul élément subsiste, l’installation s’arrête sans écraser
l’autre. Les réglages du VirtualHost de secours sont gérés depuis **Paramètres >
Accès HTTPS dédié**.

Un domaine public doit utiliser un VirtualHost Apache distinct qui transmet les
requêtes à `https://127.0.0.1:9080`. Le détail se trouve dans
`admin-https-access.md`.

## Suppression et purge

```bash
sudo apt remove aegisadmin
```

La suppression arrête les services et désactive le site Apache, mais conserve
la base, les sauvegardes, les snapshots, les configurations et la paire TLS.

```bash
sudo apt purge aegisadmin
```

La purge retire les configurations Debian, le VirtualHost dynamique et la
paire TLS. La base, ses sauvegardes et les snapshots restent volontairement
conservés. Leur suppression doit toujours être une opération manuelle,
explicite et précédée d’une sauvegarde vérifiée.

## Mise à jour depuis une copie de travail

Cette méthode est réservée au serveur de développement ou de test sur lequel le
paquet Debian a déjà créé les comptes et services :

```bash
bash backend/go/install.sh check
sudo bash backend/go/install.sh install
sudo bash backend/go/install.sh verify
```

Le dépôt Git n’est pas requis par l’installateur. Le paquet Debian reste la
méthode de distribution et d’installation initiale de référence.
