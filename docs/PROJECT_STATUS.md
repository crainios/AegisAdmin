# État du projet AegisAdmin

Dernière mise à jour : 12 septembre 2026 — version 0.2.90.

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
privilégié. Ils communiquent par un socket Unix privé. Le serveur web Go peut
fournir directement l’accès HTTPS ; lorsqu’Apache est présent, il peut publier
l’interface comme proxy inverse derrière un port dédié ou un nom de domaine.

Les migrations SQLite et la gestion de secours du compte root sont assurées
par `aegisadmin-admin`. Des exécuteurs spécialisés et validés prennent en charge
les opérations Cron et Certbot.

Les préférences d’interface liées au compte, notamment le thème, sont conservées
dans SQLite. Le cookie de thème sert au rendu immédiat et est restauré depuis le
profil lors de chaque connexion réussie.

La version 0.2.66 corrige la transmission du jeton de sécurité lors d’un
changement de thème depuis l’en-tête de l’interface Go.

La version 0.2.67 poursuit la supervision bilingue avec les écrans « Services »
et « Réseau », y compris leurs états calculés et leurs actions autorisées.

La version 0.2.68 traduit l’écran « Journaux » et ses filtres. Les lignes des
journaux restent brutes : AegisAdmin ne modifie ni les messages ni les dates
produites par les services supervisés.

La version 0.2.69 ajoute l’écran PHP à la couverture bilingue, tout en
préservant les identifiants et états techniques nécessaires au diagnostic.

La version 0.2.70 traduit l’écran MySQL/MariaDB et localise également les
séparateurs de milliers ainsi que les unités de taille de son inventaire.

La version 0.2.71 étend la traduction à Tor, y compris lorsque le logiciel est
absent et que le module doit rester consultable sans signaler une panne.

La version 0.2.72 traduit le message de validation Tor produit par le backend
et ajoute la vue principale Apache à la couverture bilingue.

La version 0.2.73 achève la traduction de l’interface Apache avec ses fenêtres
de création, modification et certificat ainsi que leurs interactions JavaScript.

La version 0.2.74 traduit les valeurs internes et actualisées du tableau de
bord ainsi que la page principale de gestion des tâches Cron.

La version 0.2.61 introduit le socle bilingue français/anglais. La langue par
défaut choisie pendant l’initialisation pilote la connexion ; après
authentification, la préférence enregistrée dans le profil devient prioritaire.
La connexion, la structure générale, la navigation et le tableau de bord sont
les premiers écrans migrés. Les modules fonctionnels seront traduits par étapes.

Depuis la version 0.2.62, root retrouve la langue globale dans « Paramètres » et
chaque utilisateur choisit sa langue personnelle dans « Mon compte ». Aucun
sélecteur de langue n’encombre l’en-tête général.

La version 0.2.63 corrige la coexistence avec une ancienne installation depuis
les sources : les migrations livrées par le paquet sous `/usr/share` sont
prioritaires sur un éventuel répertoire historique sous `/usr/local`.

La version 0.2.64 étend la traduction au parcours complet d’authentification et
au compte personnel : mot de passe obligatoire ou volontaire, défi et
activation de la double authentification, QR code et messages de validation.

La version 0.2.65 commence la supervision bilingue avec « Processus » et
« Stockage ». Elle centralise aussi le format des dates : français en
`JJ/MM/AAAA HH:mm`, anglais américain en `MM/JJ/AAAA hh:mm AM/PM`. Le journal
des accès et l’horloge de l’en-tête utilisent déjà cette convention ; les
écrans suivants l’emploieront au fil de leur migration.

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
- thèmes sombre, clair, Bootstrap et Néon Pop, avec interface responsive.

Depuis la version 0.2.30, le tableau de bord ne présente plus deux résumés de
la disponibilité : la version du système, le noyau et la durée de
fonctionnement sont regroupés dans « Informations système ». Cette durée
unique continue de progresser chaque minute dans le navigateur.

## Distribution

Le paquet Debian est publié pour `amd64` dans le dépôt APT signé officiel :

`https://packages.aegisadmin.fr`

Le guide public de vérification et d'installation est disponible sur
`https://aegisadmin.fr/`. Le paquet installe et contrôle les services systemd,
la paire TLS locale, les migrations et les droits de la base SQLite. Apache
n’est ni requis ni installé implicitement ; il n’est configuré que s’il est
déjà présent.

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

La version 0.2.23 retire de la page Mises à jour le lien technique vers le
résultat JSON. La route de suivi reste réservée à l'actualisation de la modale.

La version 0.2.24 isole l'installation des mises à jour dans
`aegisadmin-updater@.service`. Les travaux et leurs journaux sont persistés sous
`/var/lib/aegisadmin/updates` : la modale retrouve leur progression après le
redémarrage du backend et du serveur web. APT est exécuté avec son interface
automatisable `apt-get`.

Depuis la version 0.2.32, les sessions pleinement authentifiées sont aussi
conservées sous `/var/lib/aegisadmin/sessions` et restaurées après le
redémarrage du serveur web. Le fichier appartient exclusivement au compte
`aegisadmin` en mode `0600`. Les étapes anonymes, 2FA et de changement
obligatoire du mot de passe ne sont jamais restaurées.

La version 0.2.33 introduit dans Paramètres la configuration du futur service
de messagerie SMTP. Le secret est chiffré dans SQLite avec AES-GCM ; sa clé,
distincte de la base et accessible uniquement au compte `aegisadmin`, est
conservée sous `/var/lib/aegisadmin/secrets/settings.key`. Aucun envoi de
courriel n’est encore déclenché à cette étape.

La version 0.2.34 rend la présence du mot de passe SMTP visible par un masque
d’astérisques qui ne contient pas le secret et précise que le mode TLS implicite
correspond également au libellé historique SSL.

La version 0.2.35 distingue les logiciels absents des erreurs techniques. Tor
peut ainsi être absent sans rendre son écran inaccessible. La collecte
MySQL/MariaDB tolère les métriques qui diffèrent selon les versions et conserve
l’état du service lorsqu’une authentification locale empêche l’inventaire.

La version 0.2.36 automatise la création du compte MySQL/MariaDB de
supervision. Lorsque l’administration par socket n’est pas disponible,
`sudo aegisadmin mysql-setup` recueille temporairement le mot de passe
administrateur dans le terminal. Seul le secret aléatoire du compte technique
est ensuite conservé dans un fichier root en mode `0600`.

La version 0.2.37 complète cet accès avec une vue SQL SECURITY DEFINER limitée
aux noms des bases, nombres de tables et tailles. Le compte technique peut
consulter cet inventaire sans recevoir de droit global `SELECT` sur les
données applicatives.

La version 0.2.38 remplace cette vue, dont MySQL filtrait encore les lignes,
par une procédure `SQL SECURITY DEFINER`. Le résultat reste strictement limité
aux noms, nombres de tables et tailles, et le compte technique ne reçoit que le
droit d’exécuter cette procédure.

La version 0.2.39 applique à Fail2ban la détection déjà utilisée pour Tor : un
logiciel absent produit une page informative sans actions, et non une erreur
technique de chargement.

La version 0.2.40 corrige l’interprétation du résultat JSON de `composer audit`
sans vulnérabilité : la sécurité est maintenant indiquée comme saine au lieu
d’être déclarée indisponible.

La version 0.2.41 force localement HTTPS pour les dépôts publics GitHub que les
projets Composer référencent encore en SSH. La surveillance reste non
interactive et ne reçoit aucune clé SSH du serveur.

La version 0.2.42 sépare le statut de sécurité Composer de l’abandon d’un
paquet. Un paquet non maintenu ne rend plus l’audit des vulnérabilités
indisponible.

La version 0.2.43 associe un diagnostic explicite à tout audit Composer encore
indisponible afin de distinguer l’exécution, le délai et le décodage JSON.

La version 0.2.44 accepte une liste de vulnérabilités explicitement vide même
si Composer entoure son JSON d’un diagnostic parasite. Les réponses contenant
des alertes conservent une validation JSON stricte.

La version 0.2.45 préfère `/usr/local/bin/composer` en détection automatique,
afin qu’une ancienne copie installée dans `/usr/bin` ne supplante plus la
version récente utilisée par l’administrateur.

La version 0.2.46 trie les certificats Certbot par domaine et enrichit les
ports unitaires du pare-feu avec le service TCP ou UDP connu dans
`/etc/services`.

La version 0.2.47 complète cet affichage avec les profils applicatifs UFW tels
que `Apache Full` et `OpenSSH`, présentés dans la colonne « Ports / Service ».

La version 0.2.48 rend Apache facultatif. Sans Apache, le serveur Go fournit
directement HTTPS sur le port dédié et applique lui-même les adresses d’écoute
et la restriction IP. Avec Apache, le proxy inverse existant est conservé.

La version 0.2.49 ajoute le thème « Néon Pop ». Il décline l’interface avec des
accents très colorés et différencie visuellement les catégories du menu, tout
en conservant les thèmes existants et sans animation décorative permanente.

La version 0.2.50 recharge la page Mises à jour à la fermeture d’une modale
ayant installé une nouvelle version d’AegisAdmin. La liste des processus
explique également le rôle des services courants reconnus au moyen d’une
infobulle, sans attribuer de fonction arbitraire aux processus inconnus.

La version 0.2.51 rend ce rechargement immédiat après toute installation
réussie. Les infobulles de processus s’appuient désormais en priorité sur les
descriptions locales de systemd et des paquets Debian, avant le dictionnaire
interne, sans inspecter les arguments de ligne de commande.

La version 0.2.52 protège les maintenances SQLite réalisées depuis le terminal :
le service web est arrêté avant l’écriture et les droits de tous les fichiers
SQLite sont ensuite normalisés. Une limite systemd met également fin aux
boucles de démarrage après cinq échecs en une minute.

La version 0.2.53 contrôle explicitement la présence du compte root après
l’installation, signale l’initialisation obligatoire dans le terminal et sur
l’écran de connexion, et fournit la commande `aegisadmin setup-status`.
L’absence de Certbot ou de MySQL/MariaDB est désormais présentée comme un état
normal dans les modules concernés, sans masquer les véritables erreurs d’un
logiciel installé.

La version 0.2.54 désactive automatiquement l’ancien service backend lors
d’une mise à niveau. Elle empêche aussi deux processus de remplacer et
d’écouter successivement le même socket Unix, situation qui pouvait renvoyer
aléatoirement des réponses provenant d’une ancienne version.

La version 0.2.55 considère comme valide une installation Certbot sans aucun
certificat : l’absence initiale du répertoire `renewal` produit une liste vide,
alors que les erreurs de permissions et les données altérées restent bloquantes.

La version 0.2.56 permet à l’unité indépendante de mise à jour d’écrire dans
`/usr/lib/modules` pendant l’installation d’un noyau Linux. Cette exception ne
s’applique ni au serveur web ni au backend permanent.

La version 0.2.57 permet exclusivement à root de confirmer un redémarrage
immédiat ou différé lorsqu’une mise à jour installée le demande. Le backend refuse
l’opération pendant une mise à jour active et le résultat de la demande est
conservé dans le journal des accès.

La version 0.2.58 distingue le besoin courant de redémarrer du besoin futur
annoncé par une mise à jour de firmware. Seul `/var/run/reboot-required`
déclenche désormais l’alerte et les actions root.

La version 0.2.59 remplace l’affichage JSON des résultats Certbot par une
modale avec suivi automatique, synthèse en français et sortie terminal. La
liste des certificats est actualisée lorsque l’utilisateur ferme un résultat
terminé.

La version 0.2.85 fournit séparément un générateur de tableau privé pour le
serveur `packages.aegisadmin.fr`. Il agrège les téléchargements APT issus des
journaux Apache et les compteurs publics des fichiers GitHub Releases, sans
publier les adresses IP. Ce composant autonome n’est pas installé dans
AegisAdmin et dispose de sa propre protection Apache par mot de passe.

La version 0.2.86 traite explicitement le cas d’un journal ne contenant encore
aucun téléchargement de paquet : le tableau affiche zéro sans erreur de
chargement JavaScript.

La version 0.2.87 rend configurable la profondeur de lecture de l’écran
Journaux, de 1 à 5 000 lignes. Le résumé distingue le nombre de lignes retenues
par les filtres de la fenêtre demandée au backend.

La version 0.2.88 fait de la commande RAZ une remise à l’état initial complet
de l’écran Journaux : source par défaut, aucun filtre et 100 lignes.

La version 0.2.89 sécurise le redémarrage immédiat demandé depuis Mises à jour.
Une page légère est remise au navigateur avant l’arrêt, détecte la coupure puis
attend automatiquement le retour de l’endpoint de santé avant de recharger
l’interface et sa session persistante.

La version 0.2.90 complète l’écran Journaux avec le nombre total de lignes du
fichier sélectionné. Cette valeur est calculée indépendamment de la profondeur
de lecture et des filtres, puis comparée au nombre de lignes réellement
affichées.
