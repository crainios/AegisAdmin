# Documentation AegisAdmin

AegisAdmin est une application d’administration de serveurs Linux écrite en
Go. La documentation active est organisée autour de son exploitation et de son
architecture :

- `ARCHITECTURE.md` : composants et frontière de privilèges ;
- `PROJECT_STATUS.md` : état fonctionnel courant ;
- `ROADMAP.md` : prochaines étapes de stabilisation ;
- `../CHANGELOG.md` : changements de la version publique ;
- `admin-https-access.md` : écoute HTTPS et publication derrière Apache ;
- `debian-package.md` : construction et installation du paquet ;
- `operations.md` : guide opérateur consolidé, diagnostic, sauvegarde,
  restauration et récupération ;
- `debian-lifecycle-tests.md` : validation des installations, mises à niveau,
  suppressions et purges ;
- `apt-repository.md` : génération, signature, publication et configuration
  cliente du dépôt APT ;
- `tester-installation-guide.md` : contenu source de la page d’installation
  destinée au site public, non embarqué dans le paquet ;
- `tester-installation-guide.html` : version HTML prête à coller dans l’éditeur
  du site de présentation ;
- `root-password-recovery.md` : secours du compte root et de la 2FA ;
- `architecture/system-backend.md` : protocole entre le web et le daemon ;
- `server-configuration.md` : snapshots de configuration ;
- `settings.md`, `themes.md`, `two-factor-authentication.md` et
  `user-access-log.md` : fonctions de l’interface.

Les sources Go, tests et ressources intégrées se trouvent sous `backend/go`.
