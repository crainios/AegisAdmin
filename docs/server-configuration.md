# Configuration du serveur

## Étapes 1 à 3 — inventaire, snapshots et comparaison

Le module root `/configuration` utilise exclusivement le domaine Go
`configuration`. Les commandes publiques disponibles sont :

```text
configuration capabilities
configuration inventory
configuration snapshot-create
configuration snapshots
configuration snapshot IDENTIFIANT
configuration snapshot-export IDENTIFIANT
configuration snapshot-name IDENTIFIANT NOM
configuration snapshot-delete IDENTIFIANT CONFIRMATION
configuration snapshot-import NOM
configuration snapshot-compare SOURCE CIBLE
```

Le format courant est `aegisadmin.configuration.inventory.v1`. Il agrège les
collecteurs Go existants et ajoute des lectures contrôlées pour les paquets,
les unités systemd, les comptes/groupes et les ports en écoute.

Sections : système, paquets, services, comptes, réseau, ports, pare-feu,
Fail2ban, PHP et extensions, Apache et VirtualHosts, certificats,
MySQL/MariaDB et bases, Cron et Tor.

Une section indisponible ne fait pas échouer les autres. L’inventaire ne lit
jamais `/etc/shadow`, les mots de passe, les clés privées, les secrets TOTP ou
les cookies. Il ne propose aucune commande d’écriture, de correction ou
d’installation.

Root peut créer un snapshot depuis l’interface. Chaque snapshot contient
l’inventaire complet, sa date UTC et l’empreinte SHA-256 de l’inventaire. Les
fichiers sont enregistrés dans
`/var/lib/aegisadmin-system/configuration/snapshots`, avec un répertoire en
mode `0750` et des fichiers en mode `0440`. Le backend refuse les identifiants
non conformes, les liens symboliques, les fichiers trop volumineux et les
snapshots dont l’empreinte ne correspond plus au contenu.

Les snapshots peuvent recevoir un nom conservé dans une métadonnée séparée :
modifier ce nom ne modifie donc ni l’inventaire ni son empreinte. Root peut
supprimer explicitement un snapshot après confirmation de son identifiant.
Il n’existe aucune suppression automatique et aucun remplacement silencieux.

L’export repasse par la validation d’intégrité du backend. Un export provenant
d’un autre serveur peut être importé dans la limite de 32 Mio. Le contenu est
transmis au daemon par l’entrée standard, jamais comme argument de processus.
Le backend vérifie le schéma, l’identifiant et l’empreinte avant toute écriture,
refuse les doublons et marque l’origine importée dans les métadonnées.

Deux snapshots valides, locaux ou importés, peuvent être comparés par section.
Le résultat distingue les sections identiques, différentes ou absentes et
permet d’inspecter les données des deux côtés. Cette analyse reste strictement
en lecture seule.

Les étapes suivantes pourront enrichir la comparaison sémantique puis ajouter
les plans de migration et leurs corrections proposées.
