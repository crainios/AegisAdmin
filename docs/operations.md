# Guide d’exploitation AegisAdmin

Ce guide regroupe les opérations courantes de la version 0.2.x installée avec
le paquet Debian. Les commandes de développement depuis les sources sont
décrites séparément dans `backend/go/DEPLOYMENT.md`.

## Sauvegardes planifiées et transfert distant

L’écran **Cron** propose des modèles pour MySQL/MariaDB, `/etc/apache2` et les sites placés sous `/var/www`. Les archives sont produites sous `/var/backups/aegisadmin`, contrôlées, puis transférées par `rsync` sur SSH. Une archive locale n’est supprimée qu’après un transfert réussi.

La clé dédiée et l’identité du serveur distant sont conservées dans :

```text
/etc/aegisadmin-system/backup-ssh/id_ed25519
/etc/aegisadmin-system/backup-ssh/known_hosts
```

Le compte distant doit être limité au répertoire de sauvegardes. AegisAdmin impose une connexion non interactive, la vérification stricte de l’hôte et le fichier `known_hosts` dédié. Les définitions root sont placées dans `/etc/cron.d/aegisadmin-backup-*` et leurs paramètres protégés dans `/etc/aegisadmin-system/backup-tasks` ; elles ne contiennent aucun secret.

## Repères

| Élément | Emplacement ou service |
|---|---|
| Interface web Go | `aegisadmin-web.service` |
| Backend privilégié | `aegisadmin-system.service` |
| Serveur HTTPS | `aegisadmin-web.service` sur `8443`, ou proxy Apache facultatif |
| Écoute Go avec Apache | `https://127.0.0.1:9080` |
| Accès HTTPS sans Apache | port `8443` directement via Go |
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

Si MySQL ou MariaDB est actif, l’installation tente de créer automatiquement
le compte technique `aegisadmin_monitor`. Lorsque le compte administrateur SQL
exige un mot de passe, l’initialisation le demande avec une saisie masquée. La
configuration peut également être terminée ou réparée séparément :

```bash
sudo aegisadmin mysql-setup
```

Le mot de passe administrateur n’est jamais conservé. Un secret aléatoire est
généré pour le compte technique et placé dans
`/etc/aegisadmin-system/mysql-client.cnf`, fichier root en mode `0600`.
Les nombres de tables et tailles sont exposés au compte technique par la
procédure restreinte `aegisadmin_monitoring.database_inventory_v2` ; aucun
droit global de lecture des données applicatives ne lui est attribué.

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

Depuis l'interface, le module **Mises à jour** lance une unité systemd
indépendante. Elle exécute `apt-get update`, puis `apt-get -y upgrade`. La
progression est conservée sous `/var/lib/aegisadmin/updates` et réapparaît dans
la modale après le redémarrage éventuel des services AegisAdmin.

Diagnostic d'un travail, en remplaçant `IDENTIFIANT` par celui visible dans
l'URL pendant l'opération :

```bash
sudo systemctl status aegisadmin-updater@IDENTIFIANT.service --no-pager -l
sudo journalctl -u aegisadmin-updater@IDENTIFIANT.service --no-pager -l
```

Une seule mise à jour peut être exécutée à la fois.

La session authentifiée est conservée dans un stockage privé pendant le
redémarrage du serveur web. La modale peut ainsi reprendre son suivi sans
imposer une nouvelle connexion. Une session expirée, révoquée par une
modification de sécurité ou explicitement déconnectée n’est pas restaurée.

### Mise à niveau manuelle

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

Les mots de passe applicatifs, notamment SMTP, sont chiffrés dans SQLite avec
la clé privée `/var/lib/aegisadmin/secrets/settings.key`. Pour restaurer ces
secrets sur un autre serveur, sauvegarder cette clé séparément avec des droits
stricts et la restaurer en mode `0600`, propriétaire `aegisadmin`. Sans cette
clé, les données ordinaires restent restaurables mais les secrets chiffrés
doivent être saisis à nouveau.

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
Les sessions web sont en revanche supprimées afin qu’une réinstallation ne
restaure jamais une ancienne authentification.

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
