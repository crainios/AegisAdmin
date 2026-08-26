# Architecture AegisAdmin

```text
Navigateur HTTPS
       │
       ▼
aegisadmin-web (compte non privilégié)
       │ socket Unix privé
       ▼
aegisadmin-daemon (opérations système contrôlées)
       │
       ▼
Linux, systemd et outils d’administration
```

Le serveur web Go gère l’authentification, les sessions, l’interface et SQLite.
Le daemon Go expose uniquement des domaines et commandes enregistrés
explicitement. Les données persistantes résident sous `/var/lib/aegisadmin` et
les profils système sous `/etc/aegisadmin-system`.

Les ressources HTML, CSS et JavaScript principales sont intégrées au binaire
web. Seuls les thèmes et éléments graphiques volumineux restent servis depuis
`public/assets`.

Les exécuteurs Python Cron et Certbot sont des auxiliaires cloisonnés. Les
scripts shell restants sont réservés à l’installation et au packaging.
