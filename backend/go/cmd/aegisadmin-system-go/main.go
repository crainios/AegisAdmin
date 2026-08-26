package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"aegisadmin/backend/internal/api"
	"aegisadmin/backend/internal/buildinfo"
	"aegisadmin/backend/internal/client"
	"aegisadmin/backend/internal/protocol"
)

const (
	defaultSocketPath = "/run/aegisadmin-system/backend.sock"
	exitMissingDomain = 2
	exitBackendError  = 10
)

func main() {
	os.Exit(run())
}

func run() int {
	flags := flag.NewFlagSet("aegisadmin-system-go", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	socketPath := flags.String("socket", defaultSocketPath, "absolute Unix socket path")
	readInput := flags.Bool("stdin", false, "append standard input to command arguments")
	showVersion := flags.Bool("version", false, "print version")
	if err := flags.Parse(os.Args[1:]); err != nil {
		return printResponse(
			api.Failure("INVALID_ARGUMENT", "Les arguments du backend Go sont invalides."),
			exitMissingDomain,
		)
	}
	if *showVersion {
		fmt.Println(buildinfo.Version)
		return 0
	}

	arguments := flags.Args()
	if len(arguments) == 0 {
		return printResponse(
			api.Failure("MISSING_DOMAIN", "Aucun domaine n’a été indiqué."),
			exitMissingDomain,
		)
	}

	request := protocol.Request{
		Domain:    arguments[0],
		Arguments: []string{},
	}
	if len(arguments) >= 2 {
		request.Command = arguments[1]
	}
	if len(arguments) > 2 {
		request.Arguments = arguments[2:]
	}
	if *readInput {
		input, err := io.ReadAll(io.LimitReader(os.Stdin, protocol.MaximumMessageBytes+1))
		if err != nil || int64(len(input)) > protocol.MaximumMessageBytes {
			return printResponse(api.Failure("INVALID_INPUT", "Le contenu transmis au backend est invalide ou trop volumineux."), exitMissingDomain)
		}
		request.Arguments = append(request.Arguments, string(input))
	}

	reply, err := client.New(*socketPath).Execute(request)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return printResponse(
			api.Failure(
				"BACKEND_UNAVAILABLE",
				"Le service backend Go est indisponible.",
			),
			exitBackendError,
		)
	}

	return printResponse(reply.Response, reply.ExitCode)
}

func printResponse(response api.Response, exitCode int) int {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(response); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return exitBackendError
	}

	return exitCode
}
