# Accès HTTPS et proxy Apache

Le serveur Go écoute par défaut sur `https://127.0.0.1:9080`. Cette adresse est
la cible à utiliser derrière un nom de domaine Apache, par exemple
`https://admin.example.com/`. Le VirtualHost public termine sa propre connexion
HTTPS puis transmet les requêtes au serveur Go avec `ProxyPass` et
`ProxyPassReverse`.

Le port `8443` décrit ci-dessous est un accès d’administration de secours fourni
par le paquet Debian. Il ne remplace pas un VirtualHost public sur le port 443.

Le paquet Debian installe par défaut un VirtualHost Apache HTTPS dédié sur le
port `8443`. Il permet une première connexion directe avec l’adresse IP du
serveur, avant la création éventuelle d’un domaine d’administration.

## Première installation

La première installation doit être réalisée avec le paquet Debian, qui crée le
compte système, les services, la paire TLS locale et le VirtualHost de secours.
L’installateur présent dans `backend/go` est réservé aux mises à niveau depuis
une copie de travail déjà installée.

Le paquet Debian configure l’accès Apache de secours sur toutes les adresses au
port `8443`. Le certificat local permet l’accès suivant :

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

Avant de recharger Apache, l’installateur exécute son test de configuration.
Si le test ou le rechargement échoue, le VirtualHost précédent est restauré.
Le certificat et la clé existants ne sont jamais remplacés lorsqu’une seule
des deux parties est absente.

## Configuration ultérieure dans AegisAdmin

Après la première installation, root peut modifier le port HTTPS, l’adresse
d’écoute et l’adresse ou le réseau autorisé depuis **Paramètres > Accès HTTPS
dédié** (`/setting`). Le backend Go restaure le VirtualHost précédent si le
contrôle de configuration ou le rechargement d’Apache échoue.

Le pare-feu reste volontairement hors de cette opération. Il faut ouvrir le
nouveau port puis retirer l’ancien sur le serveur de test. Root peut activer ou
désactiver l’accès dédié sans perdre le port, les adresses d’écoute ni la
restriction réseau enregistrés. Avant une désactivation, un autre accès
fonctionnel doit être vérifié afin d’éviter un verrouillage accidentel.

## Ajouter ensuite un domaine

Le webmaster peut créer un VirtualHost séparé sur le port `443`, par exemple
pour `admin.example.com`, puis demander un certificat public avec Certbot. Le
site dédié sur `8443` peut rester comme accès de secours ou être désactivé
après validation du domaine.

La suppression du backend Go ne doit pas entraîner la suppression manuelle du
VirtualHost, du profil ou de la clé TLS avant d’avoir vérifié une autre voie
d’accès fonctionnelle.
