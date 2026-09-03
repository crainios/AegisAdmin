# Architecture du paquet Debian

| Usage | Chemin |
|---|---|
| Commande d’administration | `/usr/bin/aegisadmin` |
| Façade système | `/usr/bin/aegisadmin-system-go` |
| Daemon privilégié | `/usr/libexec/aegisadmin/aegisadmin-daemon` |
| Serveur web | `/usr/libexec/aegisadmin/aegisadmin-web` |
| Gestion SQLite/root | `/usr/libexec/aegisadmin/aegisadmin-admin` |
| Exécuteur de mises à jour | `/usr/libexec/aegisadmin/aegisadmin-updater` |
| Unité de mise à jour | `/usr/lib/systemd/system/aegisadmin-updater@.service` |
| Migrations | `/usr/share/aegisadmin/migrations` |
| Profils | `/etc/aegisadmin-system` |
| Base et sauvegardes | `/var/lib/aegisadmin/database` |
| Suivi des mises à jour | `/var/lib/aegisadmin/updates` |
| Socket privé | `/run/aegisadmin-system/backend.sock` |

Le serveur web fonctionne sous le compte système `aegisadmin`. Le daemon
privilégié reste séparé et accessible uniquement via le socket Unix protégé par
le groupe `aegisadmin-web`.
