# Accès HTTPS et proxy Apache

AegisAdmin fonctionne selon deux modes sélectionnés automatiquement par le
paquet. Sans Apache, le serveur Go fournit directement HTTPS sur le port
`8443`. Avec Apache, le serveur Go écoute localement sur
`https://127.0.0.1:9080` et Apache publie l’accès dédié sur `8443`. Dans les
deux cas, l’utilisateur accède initialement à
`https://adresse-ip-du-serveur:8443`.

Le port `9080` ne doit jamais être publié sur Internet. Il sert uniquement de
cible locale pour le proxy Apache.

## Première installation

La première installation doit être réalisée avec le paquet Debian, qui crée le
compte système, les services et la paire TLS locale. Il ne crée un VirtualHost
que lorsqu’Apache est déjà installé ; AegisAdmin n’installe jamais Apache.
L’installateur présent dans `backend/go` est réservé aux mises à niveau depuis
une copie de travail déjà installée.

Le paquet Debian configure l’accès dédié sur toutes les adresses au port
`8443`. Le certificat local permet l’accès suivant :

```text
https://adresse-ip-du-serveur:8443
```

Le certificat initial n’est pas signé par une autorité publique. Un
avertissement du navigateur est donc attendu. HTTP n’est pas activé afin de ne
jamais transmettre le mot de passe ou le code de double authentification sans
chiffrement.

Le port n’est pas ouvert silencieusement dans le pare-feu. Son ouverture reste
une opération distincte, disponible depuis le module Pare-feu ou directement
sur le serveur.

## Personnaliser l’écoute

Depuis `/setting`, plusieurs adresses d’écoute peuvent être indiquées, une par
ligne (seize au maximum). Les adresses IPv4 et IPv6 peuvent être combinées.
La valeur `*` couvre toutes les interfaces et doit donc être utilisée seule.

Les choix persistants sont enregistrés dans :

```text
/etc/aegisadmin-system/admin-web
```

Une mise à niveau exécutée sans option reprend ces valeurs. Le certificat et
la clé locale sont conservés sous `/etc/aegisadmin-system/tls`.

## Validation et restauration

Lorsque Apache est présent, l’installateur exécute son test de configuration
avant de le recharger. Si le test ou le rechargement échoue, le VirtualHost
précédent est restauré. Sans Apache, la configuration est appliquée au serveur
Go et son état est contrôlé.
Le certificat et la clé existants ne sont jamais remplacés lorsqu’une seule
des deux parties est absente.

## Configuration ultérieure dans AegisAdmin

Après la première installation, root peut modifier le port HTTPS, l’adresse
d’écoute et l’adresse ou le réseau autorisé depuis **Paramètres > Accès HTTPS
dédié** (`/setting`). Le backend Go applique le mode correspondant et restaure
le VirtualHost précédent si un contrôle Apache échoue.

Le pare-feu reste volontairement hors de cette opération. Il faut ouvrir le
nouveau port puis retirer l’ancien sur le serveur de test. Root peut activer ou
désactiver l’accès dédié sans perdre le port, les adresses d’écoute ni la
restriction réseau enregistrés. Avant une désactivation, un autre accès
fonctionnel doit être vérifié afin d’éviter un verrouillage accidentel.

## Ajouter ensuite un domaine

Lorsque Apache est installé, le webmaster peut créer un VirtualHost séparé sur le port `443`, par exemple
pour `admin.example.com`, puis demander un certificat public avec Certbot. Le
site dédié sur `8443` peut rester comme accès de secours ou être désactivé
après validation du domaine.

La suppression du backend Go ne doit pas entraîner la suppression manuelle du
VirtualHost, du profil ou de la clé TLS avant d’avoir vérifié une autre voie
d’accès fonctionnelle.
