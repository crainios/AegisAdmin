# AegisAdmin Go

Ce module contient les quatre exécutables AegisAdmin :

- `aegisadmin-daemon` : opérations système privilégiées via socket Unix ;
- `aegisadmin-system-go` : client d’administration en ligne de commande ;
- `aegisadmin-web` : serveur web HTTPS non privilégié ;
- `aegisadmin-admin` : migrations SQLite, mot de passe de secours et
  désactivation de secours de la double authentification root.

Validation locale :

```bash
bash backend/go/install.sh check
```

Installation ou mise à niveau :

```bash
sudo bash backend/go/install.sh install
```

La base active est `/var/lib/aegisadmin/database/aegisadmin.sqlite`. Les profils
système sont conservés sous `/etc/aegisadmin-system` lors des mises à niveau.
Le serveur web écoute par défaut en HTTPS sur `127.0.0.1:9080`.
