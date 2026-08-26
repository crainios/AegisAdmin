# Paramètres applicatifs

Le module root `/setting` centralise les valeurs par défaut propres à
AegisAdmin. Les valeurs sont conservées dans la table SQLite
`application_settings` et validées par une liste de clés applicatives connues.

Les réglages disponibles sont :

- `logs.default_source` : journal sélectionné à l’ouverture de `/logs` ;
- `certbot.default_email` : adresse proposée lors de la création d’un
  certificat depuis le module Apache.

La page permet également de télécharger une sauvegarde SQLite cohérente et de
restaurer une sauvegarde AegisAdmin compatible. La restauration vérifie
l’en-tête SQLite, `PRAGMA integrity_check`, les tables indispensables, les
empreintes de toutes les migrations présentes dans le code et l’existence
d’un unique compte root actif. Elle est refusée avant toute modification si
l’un de ces contrôles échoue.

Avant chaque restauration, la base active est sauvegardée automatiquement dans
`/var/lib/aegisadmin/database/backups`. Les dix copies de sécurité les plus récentes sont
conservées. Le remplacement final utilise un renommage atomique dans le même
système de fichiers.

Une source demandée explicitement dans l’URL de `/logs` reste prioritaire. Si
le journal enregistré n’existe plus, AegisAdmin utilise le premier journal
actuellement disponible. L’adresse Certbot reste modifiable avant chaque
demande et n’est jamais transmise automatiquement sans validation du
formulaire.

Après déploiement du code, appliquer les migrations avec l’outil d’administration :

```bash
sudo /usr/libexec/aegisadmin/aegisadmin-admin migrate
```

La table clé/valeur permet d’ajouter de futures options sans modifier son
schéma. Chaque nouvelle option doit néanmoins être déclarée, validée et testée
dans `SettingsService` avant d’être exposée à l’interface.
