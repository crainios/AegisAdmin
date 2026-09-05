# Télécharger, vérifier et installer AegisAdmin

Cette page est destinée au site public `https://aegisadmin.fr/`. Elle explique
comment télécharger, contrôler et installer AegisAdmin depuis son dépôt APT
officiel. Elle s’adresse aux personnes qui souhaitent participer aux essais
sans disposer des sources du projet.

## 1. Précautions importantes

AegisAdmin administre le système, Apache, le pare-feu, les services, les tâches
Cron, les certificats et les mises à jour. Une erreur de manipulation peut donc
rendre un serveur ou un site indisponible.

Pour un premier essai, utiliser exclusivement :

- une machine virtuelle dédiée ou un serveur de test ;
- une sauvegarde ou un snapshot restaurable ;
- un accès SSH distinct de l’interface AegisAdmin ;
- un compte autorisé à employer `sudo`.

Ne pas commencer sur un serveur de production. Ne jamais désactiver les
contrôles de signature APT et ne jamais installer une clé dont l’empreinte ne
correspond pas à celle publiée ci-dessous.

## 2. Configuration attendue

La livraison actuellement publiée est un paquet Debian `amd64`. La machine
doit utiliser un système dérivé de Debian ou Ubuntu avec systemd et APT. Apache
est facultatif et n’est jamais installé automatiquement par AegisAdmin. La
machine doit pouvoir joindre `https://packages.aegisadmin.fr` pendant
l’installation et les mises à jour.

Contrôler l’architecture et le système d’initialisation :

```bash
dpkg --print-architecture
ps -p 1 -o comm=
```

Résultats attendus :

```text
amd64
systemd
```

Mettre à jour les index existants et installer les outils nécessaires au
téléchargement et au contrôle de la clé :

```bash
sudo apt update
sudo apt install ca-certificates gnupg wget
```

## 3. Télécharger la clé publique officielle

Télécharger la clé dans un fichier temporaire, sans l’installer immédiatement :

```bash
wget --https-only \
    --output-document=/tmp/aegisadmin-archive-keyring.gpg \
    https://packages.aegisadmin.fr/aegisadmin-archive-keyring.gpg
```

Afficher son identité et ses empreintes :

```bash
gpg --show-keys --with-fingerprint \
    /tmp/aegisadmin-archive-keyring.gpg
```

La clé principale doit avoir exactement l’identité suivante :

```text
AegisAdmin Archive <packages@aegisadmin.fr>
```

Son empreinte principale officielle est :

```text
D01E 7409 36E2 CB6E 7BA8  76F9 C103 5511 5D7F 97DB
```

La sous-clé de signature initiale est :

```text
591B 7C62 F74D 090D 3B2C  E8EF CA15 4A9B 3734 C7A1
```

Pour obtenir une valeur sans espaces, plus facile à comparer :

```bash
gpg --show-keys --with-colons --fingerprint \
    /tmp/aegisadmin-archive-keyring.gpg \
    | awk -F: '$1 == "fpr" { print $10; exit }'
```

Résultat attendu :

```text
D01E740936E2CB6E7BA876F9C10355115D7F97DB
```

Arrêter immédiatement l’installation si cette valeur diffère. L’empreinte doit
idéalement être confirmée depuis le site de présentation
`https://aegisadmin.fr/` ou un autre canal indépendant du serveur de paquets.

## 4. Installer la clé et la source APT

Installer la clé publique dans le trousseau réservé à ce dépôt :

```bash
sudo install -o root -g root -m 0644 \
    /tmp/aegisadmin-archive-keyring.gpg \
    /usr/share/keyrings/aegisadmin-archive-keyring.gpg
```

Télécharger la déclaration du dépôt et l’examiner avant de l’installer :

```bash
wget --https-only \
    --output-document=/tmp/aegisadmin.sources \
    https://packages.aegisadmin.fr/aegisadmin.sources

cat /tmp/aegisadmin.sources
```

Son contenu doit correspondre à :

```text
Types: deb
URIs: https://packages.aegisadmin.fr
Suites: stable
Components: main
Architectures: amd64
Signed-By: /usr/share/keyrings/aegisadmin-archive-keyring.gpg
```

Installer ensuite cette déclaration :

```bash
sudo install -o root -g root -m 0644 \
    /tmp/aegisadmin.sources \
    /etc/apt/sources.list.d/aegisadmin.sources
```

La directive `Signed-By` limite la confiance accordée à la clé au dépôt
AegisAdmin. Ne pas utiliser `apt-key`, `trusted=yes` ou une option permettant
d’ignorer une erreur de signature.

## 5. Contrôler le paquet proposé

Actualiser les index puis afficher l’origine et la version candidate :

```bash
sudo apt update
apt policy aegisadmin
```

La table doit montrer une version candidate provenant uniquement de :

```text
https://packages.aegisadmin.fr stable/main amd64 Packages
```

APT contrôle automatiquement la signature du fichier `InRelease`, les sommes
des index puis la somme du paquet téléchargé. Une erreur de signature, de date
de validité ou de somme doit interrompre l’installation.

Pour télécharger le paquet sans l’installer :

```bash
cd /tmp
apt download aegisadmin
dpkg-deb --info aegisadmin_*_amd64.deb
```

Le champ `Package` doit valoir `aegisadmin`, l’architecture `amd64` et le
responsable `AegisAdmin <packages@aegisadmin.fr>`. Le fichier téléchargé dans
`/tmp` est facultatif : la commande d’installation suivante peut le télécharger
elle-même depuis le dépôt.

## 6. Installer AegisAdmin

Lancer l’installation depuis le dépôt signé :

```bash
sudo apt install aegisadmin
```

Le paquet installe notamment :

- le serveur web Go non privilégié ;
- le backend système Go privilégié ;
- les services systemd correspondants ;
- une écoute HTTPS Go directe sur le port `8443` lorsqu’Apache est absent, ou
  un proxy inverse Apache vers `127.0.0.1:9080` lorsqu’il est présent ;
- une paire TLS locale et une base SQLite ;
- les migrations et outils d’administration.

La fin de l’installation doit indiquer que les services ont été redémarrés. En
cas d’erreur, ne pas répéter aveuglément l’installation : consulter la section
de diagnostic avant toute modification.

## 7. Initialiser le compte administrateur

Cette opération est nécessaire uniquement lors d’une installation réellement
neuve :

```bash
sudo aegisadmin initialize
```

L’outil demande la langue par défaut (`fr` ou `en`), le prénom, le nom,
l’adresse e-mail, puis le mot de passe et sa confirmation. Cette langue pilote
l’écran de connexion et sert de valeur initiale aux comptes sans préférence.
Chaque utilisateur peut ensuite choisir sa propre langue dans **Mon compte**.
Le mot de passe comporte entre 12 et 72 caractères et apparaît sous forme de
caractères `#` pendant la frappe.

L’identifiant de connexion reste toujours :

```text
root
```

Le prénom, le nom et l’adresse e-mail ne remplacent pas cet identifiant. Ne pas
relancer l’initialisation si l’outil indique que root existe déjà.

Lorsque MySQL ou MariaDB est actif, le même parcours configure le compte de
supervision dédié. Si cette étape doit être reprise séparément :

```bash
sudo aegisadmin mysql-setup
```

Le mot de passe administrateur SQL est saisi de façon masquée et n’est pas
enregistré par AegisAdmin.

## 8. Contrôler les services

```bash
sudo systemctl status \
    aegisadmin-system.service \
    aegisadmin-web.service \
    --no-pager -l

sudo aegisadmin version
```

Avec Apache :

```bash
sudo systemctl status apache2.service --no-pager -l
wget --no-check-certificate --quiet --output-document=- \
    https://127.0.0.1:9080/readyz
```

Sans Apache :

```bash
wget --no-check-certificate --quiet --output-document=- \
    https://127.0.0.1:8443/readyz
```

Les deux services AegisAdmin doivent être actifs. Apache doit l’être uniquement
s’il est installé. La sonde doit renvoyer une réponse de
la forme :

```json
{"status":"ok","version":"VERSION","backend":"ok","database":"ok"}
```

Avec Apache, le port `9080` est l’écoute locale du serveur Go et ne doit pas
être ouvert sur Internet. Sans Apache, le serveur Go écoute directement sur le
port dédié `8443`.

## 9. Ouvrir l’interface pour la première fois

Le paquet fournit un accès HTTPS dédié :

```text
https://ADRESSE_IP_DU_SERVEUR:8443/
```

Vérifier d’abord que le serveur écoute :

```bash
sudo ss -ltnp | grep ':8443'
```

Le pare-feu n’est pas modifié automatiquement. Si le port est filtré, autoriser
uniquement l’adresse ou le réseau de test selon l’outil de pare-feu déjà utilisé
sur la machine. Éviter une ouverture mondiale sans nécessité.

Sans Apache, cet accès est servi directement par Go. Avec Apache, il est publié
par le VirtualHost dédié installé par le paquet. Le certificat initial est
local et non signé par une autorité publique. Le
navigateur affiche donc normalement un avertissement. Avant de l’accepter,
afficher son empreinte directement sur le serveur :

```bash
sudo openssl x509 \
    -in /etc/aegisadmin-system/tls/admin-local.crt \
    -noout -sha256 -fingerprint
```

Comparer cette empreinte avec celle présentée par le navigateur, puis se
connecter avec l’identifiant `root` et le mot de passe défini précédemment.

## 10. Vérifications conseillées aux testeurs

Commencer par des opérations de consultation :

1. vérifier le tableau de bord et le rafraîchissement des ressources ;
2. ouvrir Processus, Stockage, Services, Réseau et Journaux ;
3. contrôler les listes Apache, Fail2ban, Pare-feu, Cron et Certificats TLS ;
4. vérifier Mises à jour, Utilisateurs, Modules, Paramètres et Config. serveur ;
5. tester les quatre thèmes, le français, l’anglais et une largeur d’écran réduite ;
6. créer une sauvegarde depuis **Paramètres > Sauvegarde de la base**.

Ne tester les actions de modification qu’après avoir créé une sauvegarde ou un
snapshot de la machine. Noter l’heure, l’écran, l’action, le message affiché et
les éventuelles lignes de journal lorsqu’un problème apparaît.

## 11. Mise à niveau ultérieure

Les nouvelles versions sont distribuées par le même dépôt :

```bash
sudo apt update
apt policy aegisadmin
sudo apt install aegisadmin
```

La base, les sauvegardes, les snapshots, les configurations locales et la paire
TLS sont conservés lors d’une mise à niveau normale.

## 12. Diagnostic rapide

Si l’interface ne répond pas :

```bash
sudo journalctl -u aegisadmin-system.service -n 100 --no-pager -l
sudo journalctl -u aegisadmin-web.service -n 100 --no-pager -l
sudo ss -ltnp | grep -E ':8443|:9080'
```

Si Apache est installé :

```bash
sudo journalctl -u apache2.service -n 100 --no-pager -l
sudo apache2ctl configtest
```

Contrôler dans cet ordre :

1. `aegisadmin-system.service` ;
2. le socket `/run/aegisadmin-system/backend.sock` ;
3. `aegisadmin-web.service` ;
4. la sonde locale adaptée au mode d’installation ;
5. selon le mode, l’écoute Go directe sur `8443` ou Apache devant `9080` ;
6. le pare-feu et le routage réseau.

Ne jamais démarrer manuellement le serveur web en root.

## 13. Récupération du compte root

Pour remplacer le mot de passe root :

```bash
sudo /usr/libexec/aegisadmin/aegisadmin-admin reset-root-password
sudo systemctl restart aegisadmin-web.service
```

Pour désactiver sa double authentification en cas de perte du générateur TOTP :

```bash
sudo /usr/libexec/aegisadmin/aegisadmin-admin disable-root-two-factor
sudo systemctl restart aegisadmin-web.service
```

Ces opérations invalident les sessions root existantes.

## 14. Désinstallation d’une machine de test

La suppression du paquet conserve volontairement les données :

```bash
sudo apt remove aegisadmin
```

La purge retire aussi les configurations Debian et la paire TLS locale, mais
conserve encore la base, ses sauvegardes et les snapshots afin d’éviter une
destruction implicite :

```bash
sudo apt purge aegisadmin
```

Ne supprimer manuellement les données restantes qu’après avoir vérifié leur
sauvegarde et uniquement sur une machine dont la destruction est autorisée.

## 15. Informations à joindre à un rapport de test

Sans transmettre de mot de passe, clé privée, secret TOTP ou donnée personnelle,
indiquer au minimum :

```bash
cat /etc/os-release
uname -a
dpkg --print-architecture
sudo aegisadmin version
apt policy aegisadmin
```

Ajouter les étapes permettant de reproduire le problème, le résultat attendu,
le résultat observé et les extraits de journaux strictement nécessaires.
