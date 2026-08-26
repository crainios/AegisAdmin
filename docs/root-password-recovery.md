# Récupération du compte root

La commande Go d’administration travaille sur la base active située dans
`/var/lib/aegisadmin/database/aegisadmin.sqlite`.

La liste commentée des opérations disponibles s’affiche sans argument :

```bash
sudo /usr/libexec/aegisadmin/aegisadmin-admin
```

Réinitialisation du mot de passe :

```bash
sudo /usr/libexec/aegisadmin/aegisadmin-admin reset-root-password
sudo systemctl restart aegisadmin-web.service
```

Initialisation d’une installation neuve :

```bash
sudo aegisadmin initialize
```

Application manuelle des migrations :

```bash
sudo /usr/libexec/aegisadmin/aegisadmin-admin migrate
```

La réinitialisation incrémente la version d’authentification du compte et
invalide ses sessions existantes. Elle ne désactive pas sa double authentification.

Désactivation de secours de la double authentification root :

```bash
sudo /usr/libexec/aegisadmin/aegisadmin-admin disable-root-two-factor
```

Cette commande supprime le secret TOTP, retire l’obligation de double
authentification et invalide toutes les sessions root existantes. La prochaine
connexion s’effectue avec le mot de passe uniquement ; la double authentification
peut ensuite être réactivée depuis **Mon compte**.
