# Tableau privé des téléchargements

Le tableau autonome agrège les téléchargements des paquets APT depuis le
journal Apache `aegisadmin-packages-access.log*` et les compteurs publics des
fichiers joints aux GitHub Releases. Il ne copie aucune adresse IP dans le JSON
produit. Un téléchargement APT correspond à une requête `GET` réussie (`200`)
sur un fichier `aegisadmin_VERSION_ARCHITECTURE.deb` ; le résultat reste donc
un indicateur de transferts, pas un nombre garanti d’installations distinctes.

## Construire sur le poste de développement

```bash
bash packaging/download-stats/build.sh
```

Le binaire statique `dist/aegisadmin-download-stats-amd64` ne nécessite pas Go
sur le serveur de paquets.

## Préparer le mot de passe sur le serveur de paquets

```bash
sudo apt install apache2-utils
sudo htpasswd -c /etc/apache2/aegisadmin-download-stats.htpasswd VOTRE_IDENTIFIANT
sudo chown root:www-data /etc/apache2/aegisadmin-download-stats.htpasswd
sudo chmod 0640 /etc/apache2/aegisadmin-download-stats.htpasswd
```

Ne placez jamais ce fichier dans le DocumentRoot et choisissez un mot de passe
distinct de ceux d’AegisAdmin, de SSH et de GitHub.

## Transférer et installer

Depuis le poste de développement :

```bash
scp dist/aegisadmin-download-stats-amd64 SERVEUR:/tmp/
scp -r packaging/download-stats SERVEUR:/tmp/aegisadmin-download-stats-install
```

Puis sur le serveur de paquets :

```bash
sudo bash /tmp/aegisadmin-download-stats-install/install.sh \
  /tmp/aegisadmin-download-stats-amd64
```

L’installation ajoute un service ponctuel, une minuterie horaire et la
configuration Apache protégée. Le tableau devient accessible à :

```text
https://packages.aegisadmin.fr/private-downloads/
```

## Contrôler

```bash
sudo systemctl status aegisadmin-download-stats.timer --no-pager
sudo systemctl status aegisadmin-download-stats.service --no-pager
sudo journalctl -u aegisadmin-download-stats.service -n 50 --no-pager
```

Le service s’exécute comme `www-data`, reçoit seulement le groupe de lecture
des journaux `adm` et ne peut écrire que dans
`/var/lib/aegisadmin-download-stats`. Une indisponibilité de GitHub n’empêche
pas la publication des compteurs APT ; elle est signalée dans le tableau.

Les compteurs GitHub resteront à zéro tant que les paquets `.deb` ne seront pas
également joints à des GitHub Releases. Aucun jeton n’est requis pour un dépôt
public. Si la limite anonyme devient gênante, le service accepte la variable
d’environnement `GITHUB_TOKEN`, mais ce secret ne doit jamais être placé dans
le dépôt Git.

La politique de conservation et d’anonymisation du journal Apache source reste
à configurer séparément avec `logrotate`. Le tableau ne contient que des totaux
par jour, version et architecture.
