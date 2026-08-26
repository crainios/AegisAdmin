# Double authentification TOTP

AegisAdmin prend en charge les applications compatibles TOTP, dont Google
Authenticator. L’implémentation suit RFC 6238 avec HMAC-SHA1, six chiffres et
une période de trente secondes.

Root peut rendre la double authentification obligatoire pour un utilisateur
depuis `/users/{id}/edit`. Si elle n’est pas encore configurée, la prochaine
connexion correcte par mot de passe conduit à un enrôlement obligatoire. Le QR
code est généré localement dans le navigateur ; l’URI et le secret ne sont
transmis à aucun service externe.

Quand la double authentification est facultative, l’utilisateur peut l’activer
depuis `/account/password`. Il peut la désactiver au même endroit après saisie
d’un code TOTP valide. Une configuration rendue obligatoire ne peut pas être
désactivée par l’utilisateur.

Après validation du mot de passe, un compte configuré passe toujours par
`/login/two-factor`. La session temporaire expire après cinq minutes et refuse
la poursuite après huit codes invalides.

Le secret TOTP fait partie de la base SQLite. Les droits du fichier et des
sauvegardes doivent donc rester strictement limités au compte système
`aegisadmin`, au groupe `aegisadmin-web` et aux administrateurs autorisés.
