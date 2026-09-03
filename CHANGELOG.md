# Journal des versions

## 0.2.52

- Les commandes `aegisadmin initialize` et `aegisadmin migrate` arrêtent systématiquement le serveur web avant toute écriture SQLite.
- Les propriétaires et les permissions de la base, du journal WAL et de la mémoire partagée SQLite sont restaurés même lorsque la commande administrative échoue.
- Le service web limite désormais les échecs de démarrage à cinq tentatives par minute afin de ne plus saturer la console.

## 0.2.51

- La fermeture de la modale après toute installation réussie recharge immédiatement la page Mises à jour avec une URL anticache.
- Les descriptions des processus proviennent en priorité de l’unité systemd, puis du résumé du paquet Debian propriétaire de l’exécutable.
- Le dictionnaire AegisAdmin n’est plus qu’un dernier recours ciblé ; aucune ligne de commande susceptible de contenir un secret n’est consultée ni affichée.

## 0.2.50

- Après l’installation réussie d’une mise à jour contenant AegisAdmin, la fermeture de la modale recharge automatiquement la page avec la nouvelle version.
- Les processus système reconnus indiquent leur rôle dans une infobulle accessible au survol et au clavier.
- Les processus non reconnus restent affichés sans description approximative.

## 0.2.49

- Ajout du thème optionnel « Néon Pop », très coloré mais conservant des contrastes adaptés à une interface d’administration.
- Les catégories du menu reçoivent des accents cyan, violet, magenta, rouge, jaune et vert qui suivent leur ordre personnalisé.
- Les cartes, états, formulaires et terminaux disposent d’une déclinaison néon sans animation permanente.

## 0.2.48

- Apache devient facultatif dans le paquet Debian ; une installation sans Apache expose directement le serveur HTTPS Go sur le port `8443`.
- Lorsqu’Apache est déjà installé, le proxy inverse historique vers `127.0.0.1:9080` est conservé.
- La configuration de l’accès dédié pilote directement les écoutes Go et la restriction IP en l’absence d’Apache, y compris avec plusieurs adresses.
- Le module Apache affiche un état « non installé » sans rendre sa page inaccessible.

## 0.2.47

- Les règles UFW basées sur un profil applicatif affichent désormais leur cible, par exemple `Apache Full` ou `OpenSSH`, au lieu d’une cellule vide.
- La colonne du pare-feu est renommée « Ports / Service » pour réunir ports numériques enrichis et profils UFW.

## 0.2.46

- Les certificats Certbot sont triés par défaut par la colonne Domaines, puis par leur nom en cas d’égalité.
- Les ports unitaires des règles pare-feu affichent le service TCP ou UDP connu dans `/etc/services`, par exemple `80 (http)`.
- Les ports inconnus et les plages restent affichés sans qualification arbitraire.

## 0.2.45

- La détection automatique préfère désormais Composer installé dans `/usr/local/bin`, généralement plus récent que le paquet système placé dans `/usr/bin`.
- Un chemin Composer défini explicitement dans le profil reste prioritaire et inchangé.

## 0.2.44

- Le parseur d’audit Composer tolère un diagnostic parasite autour d’une réponse indiquant explicitement une liste vide de vulnérabilités.
- Une réponse contenant des alertes doit toujours rester un JSON valide afin qu’aucune vulnérabilité ne puisse être masquée.

## 0.2.43

- Une vérification de sécurité Composer indisponible expose désormais sa cause précise dans le détail du site et dans l’API.
- Le diagnostic distingue une erreur d’exécution, un délai dépassé et une réponse JSON incompatible.

## 0.2.42

- La colonne de sécurité Composer ne considère plus un paquet abandonné comme une indisponibilité de l’audit.
- Les vulnérabilités restent contrôlées et affichées indépendamment de l’état de maintenance des paquets.

## 0.2.41

- Les dépôts publics GitHub référencés par une URL SSH sont consultés en HTTPS pendant la surveillance Composer.
- Le compte système AegisAdmin n’a plus besoin d’une clé SSH ni d’une confirmation interactive de l’empreinte GitHub pour ces dépôts publics.

## 0.2.40

- La surveillance Composer reconnaît désormais la réponse JSON utilisée par `composer audit` lorsqu’aucune alerte de sécurité n’est présente.
- Un audit sain est affiché comme « Aucune alerte » au lieu de « Indisponible ».

## 0.2.39

- Un serveur sans Fail2ban affiche désormais clairement « Fail2ban n’est pas installé » au lieu d’une erreur de chargement.
- Les actions, la validation de configuration et la sélection des prisons sont masquées lorsque Fail2ban est absent.
- Les véritables erreurs d’une installation Fail2ban existante restent distinguées de l’absence du logiciel.

## 0.2.38

- Remplacement de la vue d’inventaire MySQL par une procédure `SQL SECURITY DEFINER`, car MySQL continuait à filtrer les métadonnées des bases applicatives dans la vue.
- Le compte `aegisadmin_monitor` reçoit uniquement le droit `EXECUTE` sur la procédure d’agrégation.
- La vue devenue inutile est supprimée automatiquement lors de la mise à niveau de la configuration MySQL.
- La présence de la procédure versionnée est contrôlée avant de considérer la supervision comme configurée.

## 0.2.37

- L’inventaire MySQL utilise une vue dédiée `aegisadmin_monitoring.database_inventory` pour obtenir le nombre de tables et leur taille.
- Le compte `aegisadmin_monitor` reçoit uniquement le droit de consulter cette vue, sans droit global de lecture sur les données applicatives.
- Une configuration 0.2.36 incomplète est détectée et l’assistant MySQL propose automatiquement sa mise à niveau.
- Le schéma technique d’inventaire est exclu de la liste des bases présentée dans AegisAdmin.

## 0.2.36

- L’installation prépare automatiquement un compte MySQL/MariaDB technique `aegisadmin_monitor` lorsque l’administration locale par socket est disponible.
- L’assistant `sudo aegisadmin mysql-setup` prend le relais lorsque le serveur exige un mot de passe administrateur.
- Le mot de passe technique est aléatoire, conservé dans un fichier root `0600` et n’apparaît jamais dans les arguments ni dans les journaux.
- Le backend MySQL utilise automatiquement ce profil privé et refuse les fichiers symboliques ou accessibles à un groupe ou aux autres utilisateurs.
- `sudo aegisadmin initialize` propose la configuration MySQL pendant le parcours d’une nouvelle installation.

## 0.2.35

- Un serveur sans Tor affiche désormais clairement « Tor n’est pas installé » au lieu d’une erreur de chargement.
- La supervision MySQL/MariaDB accepte l’absence de métriques propres à certaines versions sans rendre tout l’écran indisponible.
- Le service MySQL/MariaDB détecté reste affiché lorsque l’authentification locale du client est refusée, avec un avertissement explicite.
- Les collectes secondaires des métriques et des bases peuvent échouer indépendamment des informations générales.
- Le type des bases MySQL est de nouveau correctement transmis à l’interface.

## 0.2.34

- Le champ du mot de passe SMTP affiche un masque d’astérisques lorsqu’un secret est déjà enregistré, sans exposer ni renvoyer sa valeur.
- Le mode TLS implicite porte désormais le libellé « TLS implicite (SMTPS, SSL) ».
- Le texte d’aide relatif à la conservation implicite du mot de passe est retiré.

## 0.2.33

- Ajout d’une carte de configuration SMTP dans Paramètres : serveur, identifiant, mot de passe, port et mode SMTPSecure.
- Le mot de passe SMTP est chiffré en AES-GCM avant son enregistrement dans SQLite et n’est jamais renvoyé à l’interface.
- La clé de chiffrement est conservée séparément dans l’espace privé du service avec des droits `0600`.
- Un mot de passe enregistré peut être conservé lors d’une modification ou effacé explicitement.

## 0.2.32

- L’interface sélectionnée dans Réseau est maintenant signalée par un bouton vert.
- Restauration du style commun des menus déroulants d’actions, notamment dans Apache et Certbot.
- Les sessions pleinement authentifiées survivent au redémarrage du serveur web pendant une mise à jour.
- Les étapes sensibles encore incomplètes (connexion, 2FA et changement obligatoire du mot de passe) restent volontairement éphémères.

## 0.2.31

- Ajout d’un assistant de publication du dépôt APT avec configuration initiale persistante.
- Une seule commande construit, signe, contrôle, transfère et active atomiquement la version courante.
- La phrase secrète GPG reste exclusivement gérée par GnuPG et n’est jamais enregistrée.
- Le contrôle HTTPS final accepte indifféremment `curl` ou `wget`.

## 0.2.30

- Suppression du récapitulatif système redondant en tête du tableau de bord.
- La version du système, le noyau et la durée de fonctionnement sont regroupés dans la carte « Informations système ».
- La durée de fonctionnement unique continue de progresser automatiquement toutes les minutes.

## 0.2.29

- Après une mise à jour réussie, les compteurs de paquets et de sécurité ainsi que la liste des paquets disponibles sont immédiatement actualisés.
- La disponibilité du serveur affichée sur le tableau de bord progresse désormais toutes les minutes.

## 0.2.28

- La création et la modification Cron utilisent une modale commune.
- L’assistant Cron permet de sélectionner plusieurs minutes, heures, jours du mois, mois et jours de la semaine.
- Les expressions comportant des pas, plages ou macros restent modifiables sans perte grâce au mode avancé.

## 0.2.27

- Le suivi visuel d’une mise à jour survit désormais au redémarrage du serveur web et clôt correctement la modale.
- L’accès transitoire au résultat repose sur l’identifiant aléatoire du travail, expire après quatre heures et ne peut pas être mis en cache ou indexé.
- La création d’une tâche utilisateur Cron passe par une modale avec un assistant de périodicité et un mode avancé.
- Le sélecteur de la bibliothèque Cron utilise désormais les styles communs aux trois thèmes.

## 0.2.26

- La mise à jour autonome APT accepte désormais les nouvelles dépendances requises par AegisAdmin sans autoriser de suppression implicite.
- Les fichiers de configuration modifiés localement sont conservés pendant la mise à jour non interactive.
- Le compte technique `www-data` est proposé par défaut pour créer et gérer ses tâches Cron.
- La bibliothèque Cron utilise désormais un menu déroulant extensible au lieu d’ajouter une carte par modèle.

## 0.2.25

- Ajout d’une bibliothèque Cron pour sauvegarder MySQL/MariaDB, la configuration Apache et les sites sous `/var/www`.
- Ajout d’un exécuteur Go privilégié avec archives atomiques, verrouillage et rétention locale.
- Ajout du transfert hors serveur par `rsync` sur SSH avec clé dédiée et contrôle strict de l’hôte distant.

Les changements notables d'AegisAdmin sont consignés dans ce document. Le
format suit [Keep a Changelog](https://keepachangelog.com/fr/1.1.0/) et le
projet utilise une numérotation compatible avec le versionnage sémantique.

## [Non publié]

### Prévu

- poursuivre les validations fonctionnelles de la version candidate ;
- préparer la première version stable.

## [0.2.24] - 2026-08-28

### Ajouté

- service systemd indépendant pour installer les mises à jour sans appartenir
  au groupe de processus du backend AegisAdmin ;
- exécuteur dédié utilisant `apt-get update` puis `apt-get -y upgrade` ;
- suivi persistant des travaux sous `/var/lib/aegisadmin/updates`, récupérable
  après le redémarrage du backend et du serveur web ;
- verrou empêchant deux installations simultanées.

### Modifié

- la modale reconnaît l'état de préparation du service et se reconnecte après
  le redémarrage d'AegisAdmin ;
- le paquet et l'installateur source installent et contrôlent le nouvel
  exécuteur, l'unité systemd et le répertoire de suivi.

## [0.2.23] - 2026-08-27

### Modifié

- suppression du lien technique ouvrant directement le résultat JSON d'une
  mise à jour ; la progression reste présentée exclusivement dans la modale.

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

[Non publié]: https://github.com/crainios/AegisAdmin/compare/v0.2.24...HEAD
[0.2.24]: https://github.com/crainios/AegisAdmin/compare/v0.2.23...v0.2.24
[0.2.23]: https://github.com/crainios/AegisAdmin/compare/v0.2.22...v0.2.23
[0.2.22]: https://github.com/crainios/AegisAdmin/releases/tag/v0.2.22
