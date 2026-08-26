# Politique de sécurité

## Versions concernées

La série 0.2.x est une version candidate. Les corrections de sécurité sont
appliquées à la dernière version publiée dans le dépôt APT officiel. Les
versions antérieures peuvent ne plus recevoir de correction séparée.

## Signaler une vulnérabilité

N'ouvrez pas d'issue publique pour une vulnérabilité réelle ou suspectée.
Utilisez la fonction **Report a vulnerability** de l'onglet Security du dépôt
GitHub. Ce canal crée un avis de sécurité privé visible uniquement par les
responsables du projet.

Indiquez si possible :

- la version d'AegisAdmin et le système utilisé ;
- le composant et le niveau de permission concernés ;
- les conditions nécessaires à la reproduction ;
- l'impact estimé ;
- une procédure de reproduction minimale ;
- toute mesure de réduction du risque déjà identifiée.

Ne transmettez jamais de mot de passe, clé privée, secret TOTP, cookie de
session, sauvegarde de base réelle ou journal contenant des données
personnelles. Remplacez les valeurs sensibles par des exemples fictifs.

Un accusé de réception sera fourni dès que possible. Les détails ne doivent pas
être rendus publics avant la disponibilité d'une correction ou l'accord
explicite des responsables du projet.

## Installation prudente

AegisAdmin administre des composants sensibles du système. La version candidate
doit d'abord être évaluée sur une machine virtuelle ou un serveur de test,
avec une sauvegarde restaurable et un accès SSH indépendant.

Les paquets doivent provenir de `https://packages.aegisadmin.fr` et être
vérifiés avec la clé du dépôt officiel. N'utilisez jamais `trusted=yes`,
`apt-key` ou une option ignorant une erreur de signature.

