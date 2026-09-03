package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestUsageExplainsAdministrativeCommands(t *testing.T) {
	var output bytes.Buffer
	printUsage(&output, "/database.sqlite", "/migrations")
	text := output.String()
	for _, expected := range []string{
		"outil d’administration et de récupération",
		"initialize-root",
		"reset-root-password",
		"disable-root-two-factor",
		"configure-mysql",
		"/database.sqlite",
		"/migrations",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("help does not contain %q:\n%s", expected, text)
		}
	}
}
