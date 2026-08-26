# Journal des accès utilisateurs

La page root `/users/access-log` présente les événements d’authentification et
de sécurité enregistrés par AegisAdmin. Les données sont conservées dans la
table SQLite `user_access_log`, avec un horodatage UTC et l’adresse IP source.

Événements de la première version : connexions réussies ou refusées, contrôles
2FA réussis ou refusés, déconnexions, changements de mot de passe et activation
ou désactivation de la double authentification.

Le journal ne contient aucun mot de passe, code 2FA, secret TOTP ou cookie. Le
User-Agent est limité à 512 octets. Quand Apache contacte AegisAdmin depuis
l’interface locale, la dernière adresse IP valide de `X-Forwarded-For` est
utilisée. Sur une connexion directe non locale, les en-têtes de proxy sont
ignorés afin qu’un client ne puisse pas falsifier son adresse.

L’interface, réservée à root, filtre par utilisateur, adresse IP et événement,
et affiche 50 entrées par page. Une erreur d’écriture du journal ne bloque
jamais une authentification.
