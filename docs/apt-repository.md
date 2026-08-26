# Dépôt APT signé AegisAdmin

Le site public de présentation du projet est `https://aegisadmin.fr/`. Le dépôt
de paquets est publié séparément sous `https://packages.aegisadmin.fr`.

## Clé officielle de l’archive

Identité : `AegisAdmin Archive <packages@aegisadmin.fr>`

Empreinte de la clé principale :

```text
D01E 7409 36E2 CB6E 7BA8  76F9 C103 5511 5D7F 97DB
```

Sous-clé de signature initiale :

```text
591B 7C62 F74D 090D 3B2C  E8EF CA15 4A9B 3734 C7A1
```

L’empreinte principale doit être contrôlée depuis une source indépendante du
serveur de paquets. Une rotation normale de sous-clé ne modifie pas cette
empreinte principale.

Le dépôt APT publie les paquets sous une arborescence Debian `pool/` et
`dists/`. Chaque publication contient des index compressés, des accès
`by-hash`, un fichier `Release`, les signatures `InRelease` et `Release.gpg`,
ainsi que la clé publique. La clé privée ne doit jamais se trouver dans le
répertoire publié, la copie de travail ou le serveur web.

## Dépendances de publication

```bash
sudo apt install apt-utils dpkg-dev gnupg gzip xz-utils
```

Le poste de publication doit être distinct du serveur web lorsque cela est
possible. Son répertoire GnuPG doit appartenir à l’opérateur et être en mode
`0700`.

## Créer la clé d’archive

Cette opération n’est réalisée qu’une fois, sur le poste de publication :

```bash
gpg --quick-generate-key \
    "AegisAdmin Archive <packages@aegisadmin.fr>" \
    rsa4096 cert 2y

gpg --list-secret-keys --with-subkey-fingerprint
```

Noter l’empreinte complète de la clé principale, puis ajouter une sous-clé de
signature :

```bash
gpg --quick-add-key EMPREINTE_COMPLETE rsa4096 sign 1y
```

Conserver hors ligne une sauvegarde chiffrée de la clé principale et le
certificat de révocation créé par GnuPG. Le serveur de publication courant n’a
besoin que de la capacité de signature prévue par la politique retenue.

## Construire les paquets

Pour préparer en une seule opération les paquets, les index, les signatures et
les contrôles de livraison :

```bash
bash packaging/build-release.sh \
    --output dist/release-0.2.22 \
    --signing-key EMPREINTE_COMPLETE \
    --architectures amd64
```

Le résultat contient `packages/`, destiné à l’archivage des paquets construits,
et `repository/`, seul répertoire à publier. Le répertoire de sortie doit être
absent et une erreur interrompt la préparation sans laisser de livraison
partielle.

La construction détaillée reste disponible si les architectures sont préparées
séparément :

```bash
bash packaging/build-deb.sh --arch amd64 --output dist
```

Pour publier aussi `arm64`, construire le paquet correspondant depuis une
machine disposant de la chaîne Go :

```bash
bash packaging/build-deb.sh --arch arm64 --output dist
```

## Générer et signer le dépôt

Le répertoire de sortie doit être absent afin qu’une publication existante ne
puisse pas être écrasée accidentellement :

```bash
bash packaging/build-apt-repository.sh \
    --packages dist \
    --output dist/apt-repository \
    --base-url https://packages.aegisadmin.fr \
    --signing-key EMPREINTE_COMPLETE \
    --suite stable \
    --codename stable \
    --architectures amd64 \
    --valid-days 30

bash packaging/check-apt-repository.sh \
    dist/apt-repository
```

Le contrôle vérifie les deux signatures, `Acquire-By-Hash`, `Valid-Until`, les
index et leurs sommes SHA-256.

## Publication atomique

Transférer le dépôt généré et le dossier `packaging/` sur le serveur de
publication, puis employer l’outil de publication. Il contrôle la source,
effectue une copie appartenant à root, contrôle de nouveau cette copie et ne
bascule le lien Apache qu’après la réussite de toutes ces étapes :

```bash
sudo bash packaging/publish-apt-repository.sh publish \
    --repository dist/release-0.2.22/repository \
    --release 0.2.22

sudo bash packaging/publish-apt-repository.sh check
```

Une publication existante n’est jamais écrasée. Pour revenir immédiatement à
une publication antérieure encore présente :

```bash
sudo bash packaging/publish-apt-repository.sh rollback \
    --release 0.2.16
```

Le dépôt utilise un VirtualHost distinct : il ne modifie donc ni le contenu ni
la configuration du site de présentation `aegisadmin.fr`. Exemple Apache pour
le sous-domaine de paquets :

```apache
<VirtualHost *:80>
    ServerName packages.aegisadmin.fr
    Redirect permanent / https://packages.aegisadmin.fr/
</VirtualHost>

<IfModule mod_ssl.c>
<VirtualHost *:443>
    ServerName packages.aegisadmin.fr

    SSLEngine on
    SSLCertificateFile /etc/letsencrypt/live/packages.aegisadmin.fr/fullchain.pem
    SSLCertificateKeyFile /etc/letsencrypt/live/packages.aegisadmin.fr/privkey.pem

    DocumentRoot /var/www/packages.aegisadmin.fr/current

    <Directory /var/www/packages.aegisadmin.fr/current>
        Options -Indexes +FollowSymLinks
        AllowOverride None
        Require all granted
    </Directory>
</VirtualHost>
</IfModule>
```

Le VirtualHost public doit utiliser HTTPS. La navigation de répertoire reste
désactivée ; APT connaît directement les chemins des métadonnées.

La configuration prête à copier est fournie dans
`packaging/apache/aegisadmin-packages.conf`. Après vérification des chemins du
certificat :

```bash
sudo install -o root -g root -m 0644 \
    packaging/apache/aegisadmin-packages.conf \
    /etc/apache2/sites-available/aegisadmin-packages.conf
sudo a2ensite aegisadmin-packages.conf
sudo apache2ctl configtest
sudo systemctl reload apache2
```

Avant son activation, créer l’enregistrement DNS de
`packages.aegisadmin.fr`, obtenir son certificat et vérifier que les fichiers
Let’s Encrypt indiqués existent. Le site de présentation et ce VirtualHost
doivent conserver des fichiers Apache distincts.

## Configuration d’un client

Télécharger d’abord la clé par un canal HTTPS, puis contrôler son empreinte par
un second canal avant de l’installer :

```bash
curl --fail --silent --show-error \
    --output /tmp/aegisadmin-archive-keyring.gpg \
    https://packages.aegisadmin.fr/aegisadmin-archive-keyring.gpg

gpg --show-keys --with-fingerprint \
    /tmp/aegisadmin-archive-keyring.gpg

sudo install -o root -g root -m 0644 \
    /tmp/aegisadmin-archive-keyring.gpg \
    /usr/share/keyrings/aegisadmin-archive-keyring.gpg
```

La commande doit afficher l’empreinte principale officielle indiquée plus
haut. Refuser la clé si une valeur diffère.

Créer `/etc/apt/sources.list.d/aegisadmin.sources` :

```text
Types: deb
URIs: https://packages.aegisadmin.fr
Suites: stable
Components: main
Architectures: amd64
Signed-By: /usr/share/keyrings/aegisadmin-archive-keyring.gpg
```

Puis :

```bash
sudo apt update
apt policy aegisadmin
sudo apt install aegisadmin
```

Ne pas utiliser `apt-key`, l’option `trusted=yes` ou la désactivation des
contrôles de signature.

## Renouvellement et rotation

Avant l’expiration de la sous-clé de signature, la renouveler ou en créer une
nouvelle rattachée à la même clé principale, puis republier le dépôt. Pour
changer de clé principale :

1. publier la nouvelle clé publique pendant que le dépôt est encore signé par
   l’ancienne clé ;
2. laisser aux clients le temps d’installer le nouveau trousseau ;
3. signer temporairement les métadonnées selon une procédure de transition
   documentée ;
4. basculer vers la nouvelle clé et conserver l’ancienne clé publique pendant
   la période annoncée ;
5. publier immédiatement la révocation en cas de compromission.

Une clé expirée ou révoquée ne doit jamais être contournée côté client.
