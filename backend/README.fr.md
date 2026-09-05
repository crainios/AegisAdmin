# Backend AegisAdmin

Le backend actif est intégralement écrit en Go dans `backend/go`.

Les répertoires conservés à ce niveau sont :

- `config` : profils système installés sous `/etc/aegisadmin-system` ;
- `libexec` : exécuteurs Python cloisonnés utilisés par Cron et Certbot ;
- `go` : daemon privilégié, interface web, outils et installateur.

L’installation initiale doit être réalisée avec le paquet Debian. Une copie de
travail déjà installée peut ensuite être validée et mise à niveau depuis les
sources :

```bash
bash backend/go/install.sh check
sudo bash backend/go/install.sh install
sudo bash backend/go/install.sh verify
```

L’installateur source met à jour les mêmes binaires et le même service
`aegisadmin-system.service` que le paquet. Il refuse une installation si le
service web fourni par le paquet n’est pas déjà présent.
