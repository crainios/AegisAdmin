# Contribuer à AegisAdmin

La série 0.2.x est en stabilisation. Les corrections reproductibles, la
sécurité, les tests et la documentation sont prioritaires. Ne joignez aucune
donnée réelle, adresse privée, clé ou secret à une issue. Signalez les
vulnérabilités par la procédure privée de [SECURITY.fr.md](SECURITY.fr.md).

Depuis `backend/go` :

```bash
gofmt -w .
go test ./...
go vet ./...
```

Avant une modification du paquet :

```bash
bash -n backend/go/install.sh
bash packaging/build-deb.sh
bash packaging/check-deb.sh dist/aegisadmin_$(cat VERSION)_amd64.deb
```

Conservez la séparation de privilèges, validez toute entrée, respectez les
droits Consultation, Actions et Modification et ajoutez les tests adaptés.
Les textes d’interface doivent rester disponibles en français et en anglais.
