# AegisAdmin

[English documentation](README.md)

AegisAdmin est une interface web légère d’administration et de supervision
pour Debian et Ubuntu. La série 0.2.x est une version candidate destinée aux
essais sur une machine restaurable avec un accès SSH indépendant.

Elle couvre le tableau de bord, les processus, le stockage, les services, le
réseau, les journaux, Apache, PHP-FPM, MySQL/MariaDB, Tor, Fail2ban, UFW,
Certbot, Cron, les mises à jour APT et firmware, Composer, les utilisateurs,
les droits, la double authentification et les snapshots de configuration.

L’application sépare un serveur web Go non privilégié et un backend système Go
privilégié reliés par un socket Unix privé. Elle fonctionne directement en
HTTPS sur le port `8443` sans Apache. Lorsqu’Apache est présent, celui-ci sert
de proxy inverse vers `127.0.0.1:9080` ; il n’est jamais installé
implicitement.

La livraison publique actuelle cible Debian ou Ubuntu avec systemd, APT et
l’architecture `amd64`. Après installation depuis le dépôt signé officiel :

```bash
sudo apt install aegisadmin
sudo aegisadmin initialize
```

Le second paquet `arm64` peut être construit, mais doit encore être validé
avant sa publication officielle.

- Site : <https://aegisadmin.fr/>
- Paquets : <https://packages.aegisadmin.fr/>
- [Documentation française](docs/README.fr.md)
- [État du projet](docs/PROJECT_STATUS.md)
- [Politique de sécurité](SECURITY.fr.md)

Validation locale depuis `backend/go` :

```bash
go test ./...
go vet ./...
```

AegisAdmin est distribué sous licence
[GNU Affero General Public License v3.0](LICENSE).
