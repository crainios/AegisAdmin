# Journal des versions

Les changements notables d'AegisAdmin sont consignés dans ce document. Le
format suit [Keep a Changelog](https://keepachangelog.com/fr/1.1.0/) et le
projet utilise une numérotation compatible avec le versionnage sémantique.

## [Non publié]

### Prévu

- isoler les mises à jour du paquet AegisAdmin dans un service systemd dédié ;
- poursuivre les validations fonctionnelles de la version candidate ;
- préparer la première version stable.

## [0.2.22] - 2026-08-26

### Ajouté

- interface web et backend système écrits en Go ;
- supervision du tableau de bord, des ressources, processus, stockages,
  services, interfaces réseau et journaux ;
- administration d'Apache, PHP-FPM, MySQL, Tor, Fail2ban et du pare-feu ;
- gestion des tâches Cron et de leurs résultats d'exécution ;
- gestion des certificats TLS et des actions Certbot ;
- suivi des mises à jour APT, des firmwares et des dépendances Composer ;
- gestion des utilisateurs, des droits par module, de la double
  authentification et du journal d'accès ;
- paramètres, sauvegardes SQLite et snapshots comparables de la configuration ;
- trois thèmes, interface responsive et navigation adaptée aux petits écrans ;
- paquet Debian, installateur contrôlé et dépôt APT officiel signé ;
- modale de suivi des mises à jour avec présentation de type terminal.

### Sécurité

- séparation du serveur web non privilégié et du backend système par socket
  Unix privé ;
- durcissement des services systemd ;
- sessions sécurisées, protection CSRF et permissions Consultation, Actions et
  Modification ;
- commandes de récupération du mot de passe root et de la double
  authentification depuis un terminal local.

[Non publié]: https://github.com/crainios/AegisAdmin/compare/v0.2.22...HEAD
[0.2.22]: https://github.com/crainios/AegisAdmin/releases/tag/v0.2.22

