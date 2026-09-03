package mysqlsetup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratedConfigurationAndSQL(t *testing.T) {
	password, err := randomPassword()
	if err != nil || len(password) < 40 || strings.ContainsAny(password, "'\n\r") {
		t.Fatalf("randomPassword() = %q, %v", password, err)
	}
	configuration := clientConfiguration(monitorLogin, password)
	if !strings.Contains(configuration, "user=\"aegisadmin_monitor\"\n") || !strings.Contains(configuration, "password=\""+password+"\"\n") {
		t.Fatalf("unexpected configuration: %q", configuration)
	}
	statement := monitoringSQL(password)
	for _, expected := range []string{"CREATE USER IF NOT EXISTS", "ALTER USER", "DROP VIEW IF EXISTS", "CREATE PROCEDURE aegisadmin_monitoring.database_inventory_v2() SQL SECURITY DEFINER", "GRANT PROCESS", "GRANT EXECUTE ON PROCEDURE aegisadmin_monitoring.database_inventory_v2"} {
		if !strings.Contains(statement, expected) {
			t.Fatalf("missing %q in %q", expected, statement)
		}
	}
}

func TestOptionValueEscapesConfigurationSyntax(t *testing.T) {
	value := optionValue("quote\" slash\\ line\nnext")
	if value != `"quote\" slash\\ line\nnext"` {
		t.Fatalf("optionValue() = %q", value)
	}
}

func TestAtomicWriteCreatesPrivateRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "mysql-client.cnf")
	if err := atomicWrite(path, []byte("secret")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode().Perm() != 0o600 || !secureRegular(path) {
		t.Fatalf("file mode=%v secure=%v err=%v", info.Mode(), secureRegular(path), err)
	}
}

func TestSecureRegularRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if secureRegular(link) {
		t.Fatal("symlink unexpectedly accepted")
	}
}
