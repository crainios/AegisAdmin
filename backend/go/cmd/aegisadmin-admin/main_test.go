package main

import (
	"bytes"
	"os"
	"path/filepath"
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

func TestFirstExistingDirectoryKeepsPackagePriority(t *testing.T) {
	root := t.TempDir()
	packaged := filepath.Join(root, "usr-share")
	historical := filepath.Join(root, "usr-local")
	if err := os.Mkdir(packaged, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(historical, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := firstExistingDirectory(packaged, historical); got != packaged {
		t.Fatalf("migration directory=%q, want %q", got, packaged)
	}
}
