# Principes d’architecture d’AegisAdmin

Ce document décrit les règles appliquées à l’implémentation Go actuelle.

## Séparation des privilèges

`aegisadmin-web` fonctionne avec le compte non privilégié `aegisadmin`. Il gère
l’interface, les sessions et la base SQLite, mais n’exécute pas directement les
opérations d’administration du système.

Les opérations privilégiées passent par `aegisadmin-daemon` au moyen du socket
Unix `/run/aegisadmin-system/backend.sock`. Le groupe `aegisadmin-web` protège
l’accès à ce socket. Le daemon n’accepte que des domaines, commandes et
arguments explicitement enregistrés et validés.

## Responsabilités des composants

- `internal/domain/*` contient les opérations système et leur validation ;
- `internal/web*` transforme les réponses système en modèles utilisables par
  l’interface ;
- `internal/webapp` gère les routes, permissions, formulaires et rendus HTML ;
- `internal/authstore` gère SQLite, les utilisateurs, sessions et paramètres ;
- `cmd/*` assemble les dépendances et démarre les cinq exécutables.

Les ressources HTML, CSS et JavaScript principales sont intégrées dans le
binaire web. Les ressources graphiques volumineuses et les thèmes restent des
fichiers statiques servis par l’application.

## Validation stricte

Les données provenant d’un formulaire, de la base, du système ou du protocole
interne sont validées à leur frontière. Les commandes système ne sont jamais
construites à partir d’une chaîne shell libre. Les identifiants, chemins,
options et tailles autorisés sont limités avant toute opération.

L’accès à un écran et l’accès à son action d’écriture sont contrôlés séparément.
Un droit de consultation ne doit jamais exposer une action de modification.

## Persistance et secrets

La base active se trouve dans
`/var/lib/aegisadmin/database/aegisadmin.sqlite`. Les sauvegardes de sécurité
sont conservées dans `/var/lib/aegisadmin/database/backups`. Les profils système
sont stockés sous `/etc/aegisadmin-system`.

Les mots de passe, secrets TOTP, clés privées et cookies ne doivent apparaître
ni dans les journaux, ni dans les snapshots de configuration, ni dans les
arguments de processus.

## Simplicité et tests

Une abstraction est introduite lorsqu’elle réduit une duplication réelle ou
renforce une frontière de sécurité. Les interfaces Go restent petites et sont
placées près de leur consommateur.

Chaque évolution doit être couverte au niveau adapté : tests unitaires des
domaines et services, tests HTTP de l’interface, validation JavaScript et
contrôle complet avec `bash backend/go/install.sh check`.

Les scripts shell restants sont limités à l’installation et au packaging. Les
deux exécuteurs Python conservés sont cloisonnés et dédiés à Cron et Certbot.
