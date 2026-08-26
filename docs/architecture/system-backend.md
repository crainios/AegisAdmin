# Architecture du backend système

Le serveur `aegisadmin-web` s’exécute sans privilège et communique par socket
Unix avec `aegisadmin-daemon`. Seul ce daemon effectue les opérations système.

```text
Navigateur → aegisadmin-web → socket Unix → aegisadmin-daemon → système
```

La façade `aegisadmin-system-go` permet d’interroger le même daemon en ligne de
commande. Le protocole JSON impose une taille maximale, des domaines enregistrés
explicitement et une validation stricte des arguments.

Les auxiliaires non-Go sont limités aux exécuteurs Python cloisonnés de Cron et
Certbot et aux scripts d’installation et d'empaquetage.
