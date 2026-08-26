# AegisAdmin

AegisAdmin est une interface web d'administration et de supervision d'un
serveur Linux. Elle rassemble dans une interface unique les opérations
courantes tout en conservant les outils natifs du système.

La série 0.2.x est une **version candidate destinée aux essais**. Pour une
première installation, utilisez une machine virtuelle ou un serveur de test
disposant d'une sauvegarde restaurable et conservez un accès SSH indépendant.

## Fonctions principales

- tableau de bord, ressources système et processus ;
- stockage, réseau, services et journaux ;
- Apache et VirtualHosts, PHP-FPM, MySQL et Tor ;
- Fail2ban, pare-feu et certificats TLS avec Certbot ;
- tâches Cron et résultats d'exécution ;
- mises à jour APT, firmwares et dépendances Composer ;
- utilisateurs, droits par module, journal d'accès et double authentification ;
- paramètres, sauvegarde de la base et snapshots de configuration du serveur.

## Architecture et sécurité

L'application est écrite en Go et sépare deux composants :

- `aegisadmin-web`, serveur web exécuté avec un compte non privilégié ;
- `aegisadmin-daemon`, backend système privilégié accessible uniquement par un
  socket Unix privé.

Cette séparation applique le principe du moindre privilège. Les actions sont
contrôlées par domaine et par niveau d'autorisation : Consultation, Actions ou
Modification. L'application comprend également la protection CSRF, des
sessions sécurisées, la double authentification TOTP et un journal des accès.

Consultez [l'architecture détaillée](docs/ARCHITECTURE.md) et la
[politique de sécurité](SECURITY.md).

## Systèmes pris en charge

La livraison publique actuelle cible :

- Debian ou Ubuntu avec systemd et APT ;
- architecture `amd64` ;
- Apache comme frontal HTTPS.

Le code permet également de construire un paquet `arm64`, mais cette
architecture doit encore être validée avant d'être annoncée comme livraison
officielle.

## Installer la version candidate

Le paquet est distribué par le dépôt APT signé officiel :

- site : <https://aegisadmin.fr/> ;
- guide de téléchargement, contrôle et installation :
  <https://aegisadmin.fr/article/telecharger-verifier-et-installer-aegisadmin/> ;
- dépôt APT : <https://packages.aegisadmin.fr/>.

Ne désactivez jamais la vérification des signatures APT. Le guide officiel
indique comment vérifier l'empreinte de la clé avant de l'installer.

## Développer et contrôler les sources

Go 1.22 ou une version compatible est nécessaire.

```bash
cd backend/go
go test ./...
go vet ./...
```

Sur une machine Debian ou Ubuntu disposant des outils de construction :

```bash
bash packaging/build-deb.sh
bash packaging/check-deb.sh dist/aegisadmin_$(cat VERSION)_amd64.deb
```

Les scripts de déploiement modifient le système et doivent uniquement être
exécutés sur une machine de test prévue à cet effet.

## Documentation

Le [sommaire de la documentation](docs/README.md) donne accès aux guides
d'architecture, d'exploitation, de construction du paquet, de publication du
dépôt APT et de récupération du compte administrateur.

L'état de la version candidate est présenté dans
[PROJECT_STATUS.md](docs/PROJECT_STATUS.md) et les prochaines étapes dans
[ROADMAP.md](docs/ROADMAP.md).

## Contribuer et signaler un problème

Consultez [CONTRIBUTING.md](CONTRIBUTING.md) avant de proposer une modification.
Les vulnérabilités ne doivent pas être publiées dans une issue : suivez les
instructions de [SECURITY.md](SECURITY.md).

## Licence

AegisAdmin est distribué sous licence
[GNU Affero General Public License v3.0](LICENSE).

