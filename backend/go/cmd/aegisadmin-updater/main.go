package main

import (
	"flag"
	"fmt"
	"os"

	"aegisadmin/backend/internal/buildinfo"
	updatesdomain "aegisadmin/backend/internal/domain/updates"
)

func main() {
	jobID := flag.String("job", "", "identifiant du travail de mise à jour")
	stateDirectory := flag.String("state-directory", "/var/lib/aegisadmin/updates", "répertoire persistant de suivi")
	showVersion := flag.Bool("version", false, "afficher la version")
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.Version)
		return
	}
	if flag.NArg() != 0 || *jobID == "" {
		fmt.Fprintln(os.Stderr, "Erreur : un identifiant de travail est obligatoire.")
		os.Exit(2)
	}
	if err := updatesdomain.RunUpdater(*jobID, *stateDirectory); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		os.Exit(1)
	}
}
