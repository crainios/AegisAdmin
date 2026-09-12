# Journal des versions

## 0.2.89

- Un redémarrage immédiat demandé depuis Mises à jour est désormais déclenché cinq secondes plus tard afin que la page de confirmation parvienne au navigateur avant l’arrêt des services.
- Un écran d’attente bilingue constate la coupure, interroge périodiquement l’état du serveur et recharge AegisAdmin dès son retour.

## 0.2.88

- La remise à zéro de l’écran Journaux efface désormais tous les filtres et restaure l’état initial de la page, notamment le journal configuré par défaut et la fenêtre de 100 lignes.

## 0.2.87

- L’écran Journaux permet de choisir librement une fenêtre comprise entre 1 et 5 000 lignes au lieu de demander systématiquement les 100 dernières.
- Le résumé des résultats indique désormais la fenêtre sélectionnée.

## 0.2.86

- Le tableau autonome des téléchargements sérialise désormais les collections vides sous forme de listes JSON et accepte les anciennes statistiques contenant `null`. Un dépôt sans téléchargement affiche ainsi zéro au lieu d’une erreur JavaScript.

## 0.2.85

- Un tableau privé autonome agrège les téléchargements APT réussis par jour, version et architecture sans recopier les adresses IP des clients.
- Les compteurs des fichiers joints aux GitHub Releases sont fusionnés lorsqu’ils sont disponibles, sans bloquer les statistiques APT en cas d’erreur GitHub.
- Un service systemd durci, une minuterie horaire et une protection Apache par mot de passe permettent son fonctionnement sans installer AegisAdmin sur le serveur de paquets.
- Le libellé anglais de l’écran Mises à jour est uniformisé sur « Details ».

## 0.2.84

- L’anglais devient la langue par défaut du README GitHub, du guide de contribution, de la politique de sécurité et de l’index documentaire.
- Une arborescence documentaire anglaise complète est ajoutée sous `docs/en`, avec un accès explicite aux documents français conservés.
- Les deux langues sont intégrées au paquet Debian et les futures notes publiques seront rédigées en anglais en premier.

## 0.2.83

- La documentation d’installation, d’exploitation et de test distingue désormais clairement le fonctionnement autonome HTTPS et le proxy Apache facultatif.
- Les noms de services, ports, chemins persistants, étapes d’initialisation, choix de langue et configuration MySQL sont harmonisés avec le paquet actuel.
- Un guide des modules et des droits décrit les niveaux Aucun, Consultation, Actions et Modification ainsi que leur cumul.
- Le README, la feuille de route, les paramètres, les tests du cycle Debian et la version HTML du guide public ont été actualisés.

## 0.2.82

- La colonne « État / Status » de l’écran Processus traduit désormais les états Linux dynamiques selon la langue du profil.
- Le code technique de l’état (`R`, `S`, `D`, etc.) reste affiché à côté de son libellé traduit.

## 0.2.81

- L’écran « À propos » est entièrement bilingue et suit désormais la langue du profil utilisateur.
- La licence, les rôles, les contributions et les liens du projet disposent d’un rendu français ou anglais cohérent.
- L’action de déconnexion anglaise est uniformisée sur le libellé « Sign out » dans toute l’interface Go.

## 0.2.80

- Le module Configuration du serveur est bilingue pour l’inventaire, la création, l’import et la liste des snapshots.
- Les vues de détail et de comparaison traduisent leurs commandes, synthèses, états et sections sans altérer les données JSON ni les noms personnalisés.
- Les dates des snapshots et leurs tailles utilisent le format du profil français ou anglais.

## 0.2.79

- Le bouton redondant « Nouvel utilisateur » a été retiré de l’en-tête de la liste des comptes.
- L’administration des modules est bilingue, y compris le glisser-déposer, les états et les commandes, sans traduire ni altérer les noms enregistrés en base.
- L’écran Paramètres est traduit pour les préférences, SMTP, l’accès HTTPS dédié et la sauvegarde ou restauration SQLite.

## 0.2.78

- Les écrans de liste, création et modification des utilisateurs suivent désormais la langue du profil root.
- Les catégories et niveaux de droits, les états de compte et de double authentification ainsi que le générateur de mot de passe sont bilingues.
- Le journal des accès traduit ses filtres, événements, résultats et pagination tout en conservant le format de date propre à la langue.

## 0.2.77

- La catégorie de navigation « Sécurité » est maintenant rendue « Security » pour les profils anglais, y compris dans le menu chargé dynamiquement.
- L’écran Mises à jour est bilingue pour les paquets, firmwares, dépendances Composer et demandes de redémarrage.
- La fenêtre terminal de mise à jour traduit ses états et utilise le format de date français ou américain selon le profil.

## 0.2.76

- L’écran Fail2ban est bilingue pour le service, la configuration, les prisons, les statistiques et les adresses bannies.
- Les actions de rechargement, redémarrage, bannissement et débannissement suivent la langue du profil sans modifier les niveaux de droits existants.
- L’écran Pare-feu, l’ajout et la suppression de règles ainsi que les confirmations d’activation ou de désactivation sont désormais traduits.

## 0.2.75

- La bibliothèque de tâches Cron, ses formulaires, confirmations et résultats d’exécution suivent désormais la langue du profil.
- L’écran Certbot est bilingue, y compris l’absence de Certbot, les informations du minuteur, les certificats, les états et les actions.
- La fenêtre de résultat Certbot et les confirmations JavaScript sont traduites ; les durées utilisent le format numérique français ou anglais.

## 0.2.74

- Les contenus dynamiques des cartes du tableau de bord sont désormais traduits, y compris après leur actualisation automatique.
- Les informations système, valeurs de ressources, états de supervision, disponibilité et durée de fonctionnement suivent la langue du profil.
- La page principale Cron, ses informations de service, son tableau, ses états et ses actions respectent maintenant le français ou l’anglais.

## 0.2.73

- Les fenêtres Apache de création de site, édition de VirtualHost et demande de certificat sont désormais entièrement bilingues.
- Le bouton « Ajouter un site », les sélecteurs, champs et boutons de validation suivent la langue du profil.
- Les messages JavaScript de chargement et de confirmation de suppression Apache sont également traduits.

## 0.2.72

- Le message dynamique confirmant la validité de la configuration Tor est maintenant traduit en anglais.
- La vue principale Apache suit la langue du profil : installation, configuration, synthèse, sites, VirtualHosts, états et commandes du service.
- Les identifiants de fichiers, domaines, ports, DocumentRoot et diagnostics Apache restent inchangés pour préserver leur valeur technique.

## 0.2.71

- L’écran Tor suit désormais la langue du profil pour l’installation, la configuration, l’état de l’instance, les services Onion et les actions.
- L’absence normale de Tor dispose également d’une présentation complète en français et en anglais.
- Les tailles et nombres affichés utilisent le format de la langue, sans modifier les adresses Onion, unités systemd ni messages techniques de configuration.

## 0.2.70

- L’écran MySQL/MariaDB suit désormais la langue du profil pour les informations serveur, métriques, bases, avertissements connus et actions.
- Les types de bases sont traduits, tandis que leurs noms et les états techniques bruts restent inchangés.
- Les grands nombres utilisent l’espace fine en français et la virgule en anglais ; les tailles anglaises emploient B, KiB, MiB, GiB et TiB.

## 0.2.69

- L’écran PHP suit désormais la langue du profil pour la présentation de PHP CLI, la liste PHP-FPM, les états et les actions.
- Les états dynamiques des instances et les valeurs Oui/Non sont traduits sans modifier les noms de services ni les états techniques bruts.
- Un test contrôle le rendu anglais et confirme qu’un utilisateur en consultation ne voit toujours pas le bouton de redémarrage.

## 0.2.68

- L’écran « Journaux » suit désormais la langue du profil pour ses filtres, ses actions, ses états vides et le décompte des résultats.
- Le singulier et le pluriel du nombre de lignes sont rendus correctement en français et en anglais.
- Le contenu brut des journaux reste volontairement inchangé afin de préserver fidèlement les messages et horodatages émis par les services du serveur.

## 0.2.67

- Les écrans « Services » et « Réseau » suivent désormais la langue du profil dans leurs titres, tableaux, synthèses, détails et états.
- Les libellés dynamiques des services et interfaces, les boutons d’action ainsi que les valeurs Oui/Non sont traduits côté serveur.
- Des tests vérifient le rendu anglais et préservent l’absence d’actions pour un utilisateur limité à la consultation.

## 0.2.66

- Le changement de thème réutilise désormais correctement le jeton de sécurité après le déplacement du formulaire de déconnexion dans le menu latéral.
- Un test protège ce lien entre le sélecteur de thème et le jeton de la session afin d’éviter le retour de l’erreur d’enregistrement du profil.

## 0.2.65

- Les écrans « Processus » et « Stockage » suivent désormais la langue du profil, y compris leurs titres, tableaux, synthèses et états générés côté serveur.
- Un formateur central applique `JJ/MM/AAAA HH:mm` en français et le format américain `MM/JJ/AAAA hh:mm AM/PM` en anglais.
- Le journal des accès utilise ce format localisé et l’horloge de l’en-tête emploie maintenant `fr-FR` ou `en-US` selon le profil.
- Des tests contrôlent explicitement le stockage en anglais et les deux formats de date.

## 0.2.64

- Le parcours d’authentification est désormais bilingue de bout en bout : changement obligatoire du mot de passe, défi 2FA et configuration par QR code.
- La page « Mon compte », son formulaire de langue, le changement de mot de passe et les états d’activation 2FA suivent la préférence du profil.
- Les messages de validation de mot de passe et de codes d’authentification disposent maintenant de traductions françaises et anglaises contrôlées.

## 0.2.63

- L’outil de migration privilégie désormais le répertoire du paquet Debian `/usr/share/aegisadmin/migrations` devant l’ancien emplacement historique `/usr/local/share/aegisadmin/migrations`.
- Une ancienne installation depuis les sources ne peut ainsi plus masquer les migrations récentes du profil utilisateur, notamment celles du thème et de la langue.

## 0.2.62

- Le sélecteur de langue quitte l’en-tête général et se trouve désormais dans « Mon compte » pour la préférence personnelle de chaque utilisateur.
- Root peut choisir dans « Paramètres » la langue par défaut de l’interface et de la page de connexion.
- La langue globale est enregistrée dans les paramètres SQLite et prend effet sans redémarrage ; la préférence personnelle et son cookie restent prioritaires lorsqu’ils existent.

## 0.2.61

- Le socle bilingue français/anglais repose sur deux catalogues intégrés et contrôlés au démarrage.
- Une installation neuve utilise le français par défaut ; `aegisadmin initialize` permet de choisir `fr` ou `en` pour les écrans anonymes, sans rendre les mises à niveau interactives.
- La page de connexion, le tableau de bord et la navigation générale disposent de leurs premières traductions anglaises.
- Chaque utilisateur peut choisir sa langue dans l’en-tête ; le choix est enregistré dans son profil SQLite, restauré à la connexion et synchronisé dans un cookie d’affichage.

## 0.2.60

- Le thème choisi est désormais enregistré dans le profil utilisateur et partagé entre ses navigateurs après connexion.
- La connexion restaure le thème du profil dans un cookie d’affichage ; vider les données du navigateur ne perd donc plus le choix enregistré.
- Un échec d’enregistrement annule immédiatement le changement visuel et avertit l’utilisateur.

## 0.2.59

- Le résultat d’une action Certbot s’affiche désormais dans une modale lisible au lieu d’ouvrir directement sa représentation JSON.
- La modale suit automatiquement l’action en cours et présente son libellé, son état, sa durée, son code retour ainsi que les sorties Certbot dans un terminal.
- La fermeture d’un résultat terminé recharge la liste des certificats ; un accès direct à l’ancienne URL revient vers la page Certbot et ouvre la même modale.

## 0.2.58

- L’indication `needs-reboot` d’une mise à jour de firmware décrit désormais uniquement son installation future et ne déclenche plus prématurément l’alerte de redémarrage du serveur.
- L’alerte et les actions root reposent exclusivement sur l’état courant `/var/run/reboot-required`, qui disparaît effectivement après un redémarrage.

## 0.2.57

- Lorsqu’un redémarrage est requis, root peut redémarrer immédiatement le serveur ou programmer l’opération dans 5, 15, 30 ou 60 minutes.
- Une confirmation explicite est obligatoire et toute demande est refusée tant qu’une mise à jour de paquets est active.
- La programmation, réussie ou refusée, est inscrite dans le journal des accès avec l’utilisateur, l’adresse IP et l’horodatage.

## 0.2.56

- L’exécuteur de mises à jour autorise désormais l’écriture légitime dans `/usr/lib/modules`, nécessaire au dépaquetage et à la configuration des nouveaux noyaux Linux.
- Les protections du serveur web et du backend permanent restent inchangées.

## 0.2.55

- Une installation Certbot ne possédant encore aucun certificat retourne désormais une liste vide au lieu d’une erreur de lecture.
- Les erreurs de permissions, les chemins invalides et les certificats réellement illisibles restent signalés.

## 0.2.54

- La mise à niveau arrête et désactive l’ancien service `aegisadmin-daemon.service`, qui pouvait répondre sur le même socket que le backend désormais fourni par le paquet.
- Le backend refuse de remplacer le socket Unix d’un serveur encore actif ; seuls les sockets réellement abandonnés sont nettoyés au démarrage.

## 0.2.53

- La fin d’une première installation signale clairement que la création du compte root est obligatoire, d’après l’état réel de la base et non sa seule présence.
- La commande `sudo aegisadmin setup-status` permet de vérifier à tout moment si cette initialisation est terminée ; l’écran de connexion indique aussi la commande à exécuter tant que root est absent.
- Les modules Certbot et MySQL/MariaDB affichent désormais un état « non installé » lorsque le logiciel correspondant est absent, sans présenter cette situation normale comme une panne.

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
