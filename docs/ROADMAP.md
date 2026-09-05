# Feuille de route AegisAdmin

## État actuel

La version 0.2.83 est la version candidate courante :

- interface web et backend système en Go ;
- seize domaines d'administration et de supervision ;
- paquet Debian et cycle de vie contrôlé ;
- dépôt APT officiel signé ;
- documentation d'installation et d'exploitation ;
- quatre thèmes, interface responsive et choix français ou anglais par profil ;
- bibliothèque de sauvegardes Cron avec transfert `rsync`/SSH hors serveur ;
- suivi des mises à jour dans une modale de type terminal.
- mise à jour autonome du paquet par un service systemd indépendant avec suivi
  persistant.
- configuration SMTP avec stockage chiffré du mot de passe, en préparation des
  notifications par courriel.

## Série 0.2.x — Stabilisation

1. valider la mise à jour autonome du paquet sur le serveur de test ;
2. poursuivre les validations fonctionnelles sur le serveur de test ;
3. corriger les anomalies découvertes à l'usage ;
4. automatiser les tests Go et les contrôles de paquet sur GitHub ;
5. valider, signer et documenter chaque nouvelle livraison candidate.
6. ajouter le test SMTP puis les notifications configurables.

## Première version stable

La première version stable sera préparée après validation des parcours
principaux : installation, authentification, consultation, actions système,
sauvegarde, restauration et mise à jour.

La stabilité annoncée concernera d'abord les systèmes Debian ou Ubuntu avec
systemd, APT et l'architecture `amd64`. Apache sera facultatif.

## Évolutions ultérieures

- validation et publication officielle des paquets `arm64` ;
- définition d'une interface publique d'extension avant toute ouverture à des
  modules tiers ;
- amélioration de la gestion coordonnée de plusieurs serveurs ;
- définition du rythme de renouvellement des sous-clés de signature APT ;
- enrichissement progressif des domaines de supervision et d'administration.
