package main

import (
	"aegisadmin/backend/internal/backup"
	"aegisadmin/backend/internal/buildinfo"
	"flag"
	"fmt"
	"os"
)

func main() {
	dir := flag.String("config-directory", "/etc/aegisadmin-system/backup-tasks", "répertoire des tâches")
	version := flag.Bool("version", false, "afficher la version")
	flag.Parse()
	if *version {
		fmt.Println(buildinfo.Version)
		return
	}
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "Erreur : identifiant de sauvegarde obligatoire.")
		os.Exit(2)
	}
	if err := backup.Run(flag.Arg(0), *dir); err != nil {
		fmt.Fprintf(os.Stderr, "Erreur : %v\n", err)
		os.Exit(1)
	}
}
