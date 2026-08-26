package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"aegisadmin/backend/internal/authstore"
	"aegisadmin/backend/internal/buildinfo"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Erreur :", err)
		os.Exit(1)
	}
}

func run() error {
	flags := flag.NewFlagSet("aegisadmin-admin", flag.ContinueOnError)
	database := flags.String("database", "/var/lib/aegisadmin/database/aegisadmin.sqlite", "chemin de la base SQLite")
	migrations := flags.String("migrations", defaultMigrationsDirectory(), "répertoire des migrations SQL")
	version := flags.Bool("version", false, "afficher la version")
	flags.Usage = func() { printUsage(flags.Output(), *database, *migrations) }
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *version {
		fmt.Println(buildinfo.Version)
		return nil
	}
	if flags.NArg() == 0 {
		printUsage(os.Stdout, *database, *migrations)
		return nil
	}
	if flags.NArg() != 1 {
		flags.Usage()
		return errors.New("une seule commande doit être indiquée")
	}
	command := flags.Arg(0)
	if command == "help" {
		printUsage(os.Stdout, *database, *migrations)
		return nil
	}
	validCommands := map[string]bool{"migrate": true, "initialize-root": true, "reset-root-password": true, "disable-root-two-factor": true}
	if !validCommands[command] {
		flags.Usage()
		return fmt.Errorf("commande inconnue : %s", command)
	}
	migrationContext, cancelMigration := context.WithTimeout(context.Background(), 30*time.Second)
	applied, err := authstore.Migrate(migrationContext, *database, *migrations)
	cancelMigration()
	if err != nil {
		return err
	}
	if command == "migrate" {
		if len(applied) == 0 {
			fmt.Println("Aucune migration à appliquer.")
		} else {
			for _, name := range applied {
				fmt.Println("Migration appliquée :", name)
			}
		}
		return nil
	}
	store, err := authstore.OpenReadWrite(*database)
	if err != nil {
		return err
	}
	defer store.Close()
	scanner := bufio.NewReader(os.Stdin)
	switch command {
	case "initialize-root":
		last, err := readValue(scanner, "Nom : ")
		if err != nil {
			return err
		}
		first, err := readValue(scanner, "Prénom : ")
		if err != nil {
			return err
		}
		email, err := readValue(scanner, "Adresse e-mail : ")
		if err != nil {
			return err
		}
		password, err := readConfirmedPassword(scanner)
		if err != nil {
			return err
		}
		operationContext, cancelOperation := context.WithTimeout(context.Background(), 30*time.Second)
		err = store.InitializeRoot(operationContext, first, last, email, password)
		cancelOperation()
		if err != nil {
			return err
		}
		fmt.Println("Le compte root a été initialisé.")
	case "reset-root-password":
		password, err := readConfirmedPassword(scanner)
		if err != nil {
			return err
		}
		operationContext, cancelOperation := context.WithTimeout(context.Background(), 30*time.Second)
		err = store.ResetRootPassword(operationContext, password)
		cancelOperation()
		if err != nil {
			return err
		}
		fmt.Println("Le mot de passe root a été régénéré et ses sessions ont été invalidées.")
	case "disable-root-two-factor":
		operationContext, cancelOperation := context.WithTimeout(context.Background(), 30*time.Second)
		err = store.DisableRootTwoFactor(operationContext)
		cancelOperation()
		if err != nil {
			return err
		}
		fmt.Println("La double authentification root a été désactivée et ses sessions ont été invalidées.")
	}
	return nil
}

func printUsage(output io.Writer, database, migrations string) {
	fmt.Fprintf(output, `AegisAdmin — outil d’administration et de récupération

Utilisation :
  sudo aegisadmin-admin COMMANDE
  sudo aegisadmin-admin [OPTIONS] COMMANDE

Commandes :
  migrate                    Appliquer les migrations SQLite en attente.
  initialize-root            Initialiser le compte root d’une nouvelle installation.
  reset-root-password        Définir un nouveau mot de passe root et fermer ses sessions.
  disable-root-two-factor    Désactiver la 2FA root et fermer ses sessions.
  help                       Afficher cette aide.

Options :
  --database CHEMIN          Base SQLite à utiliser.
                             Valeur par défaut : %s
  --migrations RÉPERTOIRE    Répertoire contenant les migrations SQL.
                             Valeur détectée : %s
  --version                  Afficher la version installée.
  -h, --help                 Afficher cette aide.

Exemples :
  sudo aegisadmin-admin reset-root-password
  sudo aegisadmin-admin disable-root-two-factor
  sudo aegisadmin-admin migrate
`, database, migrations)
}

func defaultMigrationsDirectory() string {
	const local = "/usr/local/share/aegisadmin/migrations"
	if info, err := os.Stat(local); err == nil && info.IsDir() {
		return local
	}
	return "/usr/share/aegisadmin/migrations"
}

func readValue(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	value, err := reader.ReadString('\n')
	value = strings.TrimSpace(value)
	if err != nil || value == "" {
		return "", errors.New("information obligatoire")
	}
	return value, nil
}
func readConfirmedPassword(reader *bufio.Reader) (string, error) {
	first, err := readPassword(reader, "Nouveau mot de passe : ")
	if err != nil {
		return "", err
	}
	second, err := readPassword(reader, "Confirmation : ")
	if err != nil {
		return "", err
	}
	if first != second {
		return "", errors.New("les deux mots de passe ne correspondent pas")
	}
	return first, nil
}
func readPassword(reader *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	stateCommand := exec.Command("/usr/bin/stty", "-g")
	stateCommand.Stdin = os.Stdin
	stateOutput, err := stateCommand.Output()
	if err != nil {
		return "", errors.New("la saisie sécurisée du mot de passe nécessite un terminal interactif")
	}
	state := strings.TrimSpace(string(stateOutput))
	setTerminal := func(arguments ...string) error {
		command := exec.Command("/usr/bin/stty", arguments...)
		command.Stdin = os.Stdin
		return command.Run()
	}
	if err = setTerminal("-echo", "-icanon", "-isig", "min", "1", "time", "0"); err != nil {
		return "", errors.New("le terminal ne peut pas activer la saisie masquée")
	}
	defer func() { _ = setTerminal(state) }()

	value := make([]byte, 0, 72)
	for {
		character, readErr := reader.ReadByte()
		if readErr != nil {
			fmt.Println()
			return "", readErr
		}
		switch character {
		case '\r', '\n':
			fmt.Println()
			return string(value), nil
		case 3, 4, 26:
			fmt.Println()
			return "", errors.New("saisie du mot de passe interrompue")
		case 8, 127:
			if len(value) > 0 {
				_, size := utf8.DecodeLastRune(value)
				if size < 1 {
					size = 1
				}
				value = value[:len(value)-size]
				fmt.Print("\b \b")
			}
		case 21:
			for len(value) > 0 {
				_, size := utf8.DecodeLastRune(value)
				if size < 1 {
					size = 1
				}
				value = value[:len(value)-size]
				fmt.Print("\b \b")
			}
		default:
			value = append(value, character)
			fmt.Print("#")
		}
	}
}
