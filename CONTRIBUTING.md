# Contribuer à AegisAdmin

Merci de votre intérêt pour AegisAdmin. La série 0.2.x est en phase de
stabilisation : les corrections reproductibles, les améliorations de sécurité,
les tests et les clarifications documentaires sont prioritaires.

## Avant de commencer

- recherchez si une issue décrit déjà le problème ;
- n'incluez aucune donnée réelle de serveur, adresse privée, clé ou secret ;
- pour une vulnérabilité, utilisez exclusivement la procédure privée décrite
  dans [SECURITY.md](SECURITY.md) ;
- testez toute action système sur une machine isolée et restaurable.

## Organisation des sources

- `backend/go/` : serveur web, backend système et tests Go ;
- `backend/config/` : profils système installés ;
- `backend/libexec/` : exécuteurs sécurisés Cron et Certbot ;
- `database/migrations/` : migrations SQLite ;
- `packaging/` : paquet Debian et dépôt APT ;
- `docs/` : architecture et exploitation ;
- `public/assets/` : ressources graphiques partagées.

## Contrôles locaux

Depuis `backend/go` :

```bash
gofmt -w .
go test ./...
go vet ./...
```

Avant de proposer une modification de l'empaquetage, depuis la racine :

```bash
bash -n backend/go/install.sh
bash packaging/build-deb.sh
bash packaging/check-deb.sh dist/aegisadmin_$(cat VERSION)_amd64.deb
```

La construction du paquet nécessite notamment Go, `rsync`, `dpkg-deb` et les
outils usuels d'une distribution Debian ou Ubuntu.

## Principes de contribution

- conserver la séparation entre le serveur web non privilégié et le backend ;
- valider strictement toutes les données avant une action système ;
- éviter l'exécution par shell lorsqu'une invocation argumentée est possible ;
- respecter les niveaux Consultation, Actions et Modification ;
- ajouter ou adapter les tests pour chaque comportement modifié ;
- conserver les textes d'interface en français et accessibles ;
- mettre à jour la documentation et `CHANGELOG.md` lorsque nécessaire.

Une proposition doit rester ciblée, expliquer le résultat attendu et indiquer
les contrôles exécutés. En contribuant, vous acceptez que votre contribution
soit distribuée sous la licence GNU AGPL v3.0 du projet.

