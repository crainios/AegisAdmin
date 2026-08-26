# Feuille de route AegisAdmin

## État actuel

La version 0.2.22 est une version candidate publique :

- interface web et backend système en Go ;
- seize domaines d'administration et de supervision ;
- paquet Debian et cycle de vie contrôlé ;
- dépôt APT officiel signé ;
- documentation d'installation et d'exploitation ;
- audit des trois thèmes, contrôle responsive et harmonisation des permissions ;
- suivi des mises à jour dans une modale de type terminal.

## Série 0.2.x — Stabilisation

1. isoler l'installation des mises à jour dans un service systemd dédié afin
   qu'AegisAdmin puisse mettre à jour son propre paquet sans interrompre APT ;
2. poursuivre les validations fonctionnelles sur le serveur de test ;
3. corriger les anomalies découvertes à l'usage ;
4. automatiser les tests Go et les contrôles de paquet sur GitHub ;
5. valider, signer et documenter chaque nouvelle livraison candidate.

## Première version stable

La première version stable sera préparée après validation des parcours
principaux : installation, authentification, consultation, actions système,
sauvegarde, restauration et mise à jour.

La stabilité annoncée concernera d'abord les systèmes Debian ou Ubuntu avec
systemd, APT, Apache et l'architecture `amd64`.

## Évolutions ultérieures

- validation et publication officielle des paquets `arm64` ;
- définition d'une interface publique d'extension avant toute ouverture à des
  modules tiers ;
- amélioration de la gestion coordonnée de plusieurs serveurs ;
- définition du rythme de renouvellement des sous-clés de signature APT ;
- enrichissement progressif des domaines de supervision et d'administration.

