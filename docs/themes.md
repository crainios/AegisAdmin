# Thèmes de l’interface

Les thèmes sont définis dans `public/assets/css/themes.css` et sélectionnés par
le menu de l’interface Go. Le choix est enregistré localement dans le navigateur.

Chaque thème doit uniquement surcharger les variables CSS communes. Après une
modification, exécuter :

```bash
bash backend/go/install.sh check
```

Puis vérifier au minimum les écrans de connexion, le tableau de bord, les
tableaux, formulaires, fenêtres modales et badges d’état avec chaque thème.
