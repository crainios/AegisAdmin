# État du projet AegisAdmin

Dernière mise à jour : 26 août 2026 — version 0.2.22.

## Statut

AegisAdmin 0.2.x est une première version candidate complète. Les fonctions
principales, l'interface, l'empaquetage Debian et le dépôt APT signé sont
opérationnels. Les validations fonctionnelles se poursuivent sur un serveur de
test avant la première version stable.

La version candidate ne doit pas être évaluée pour la première fois sur un
serveur de production. Une machine restaurable et un accès SSH indépendant sont
recommandés.

## Architecture

AegisAdmin utilise un serveur web Go non privilégié et un backend système Go
privilégié. Ils communiquent par un socket Unix privé. Apache fournit l'accès
HTTPS et peut publier l'interface derrière un nom de domaine.

Les migrations SQLite et la gestion de secours du compte root sont assurées
par `aegisadmin-admin`. Des exécuteurs spécialisés et validés prennent en charge
les opérations Cron et Certbot.

## Fonctions disponibles

- tableau de bord, ressources, processus, stockage, services, réseau et logs ;
- PHP-FPM, MySQL, Tor, Apache, Fail2ban et pare-feu ;
- sites Apache et VirtualHosts ;
- Cron et résultats d'exécution ;
- certificats TLS et actions Certbot ;
- mises à jour APT, firmware et dépendances Composer des sites administrés ;
- utilisateurs, compte personnel, double authentification et journal d'accès ;
- droits Consultation, Actions et Modification par module ;
- organisation des modules, paramètres et sauvegardes SQLite ;
- inventaires, snapshots et comparaison structurée de la configuration serveur ;
- thèmes sombre, clair et Bootstrap, avec interface responsive.

## Distribution

Le paquet Debian est publié pour `amd64` dans le dépôt APT signé officiel :

`https://packages.aegisadmin.fr`

Le guide public de vérification et d'installation est disponible sur
`https://aegisadmin.fr/`. Le paquet installe et contrôle les services systemd,
Apache, la paire TLS locale, les migrations et les droits de la base SQLite.

Le cycle installation, réinstallation, mise à niveau, suppression et purge a
été validé sur le serveur de test.

## Qualité et stabilisation

- documentation d'exploitation consolidée ;
- audit des thèmes et contrôle responsive terminés ;
- composants d'interface harmonisés ;
- permissions Consultation, Actions et Modification contrôlées ;
- construction et structure du paquet vérifiées automatiquement ;
- publication atomique et contrôle cryptographique du dépôt APT ;
- suivi en temps réel des mises à jour dans une modale de type terminal.

## Limite connue

AegisAdmin détecte son propre paquet parmi les mises à jour APT, mais sa mise à
jour automatique doit encore être isolée dans un service systemd indépendant.
En attendant, une mise à jour d'AegisAdmin doit être lancée depuis un terminal
avec APT afin que le redémarrage des services ne coupe pas le processus qui
effectue l'installation.

