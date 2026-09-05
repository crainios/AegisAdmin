package authstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLifecycleMigratesAndManagesRoot(t *testing.T) {
	directory := t.TempDir()
	migrations := filepath.Join(directory, "migrations")
	if err := os.Mkdir(migrations, 0o755); err != nil {
		t.Fatal(err)
	}
	schema := `CREATE TABLE users (id INTEGER PRIMARY KEY,login TEXT UNIQUE,password_hash TEXT,type TEXT,status TEXT,can_view INTEGER,can_act INTEGER,can_modify INTEGER,must_change_password INTEGER,auth_version INTEGER,created_at TEXT,updated_at TEXT,password_changed_at TEXT,first_name TEXT,last_name TEXT,email TEXT,two_factor_required INTEGER,totp_secret TEXT,totp_enabled_at TEXT);`
	if err := os.WriteFile(filepath.Join(migrations, "001_users.sql"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}
	database := filepath.Join(directory, "aegis admin.sqlite")
	initialized, err := RootInitialized(context.Background(), database)
	if err != nil || initialized {
		t.Fatalf("missing database initialized=%t err=%v", initialized, err)
	}
	applied, err := Migrate(context.Background(), database, migrations)
	if err != nil || len(applied) != 1 {
		t.Fatalf("migrate=%v err=%v", applied, err)
	}
	initialized, err = RootInitialized(context.Background(), database)
	if err != nil || initialized {
		t.Fatalf("migrated database initialized=%t err=%v", initialized, err)
	}
	store, err := OpenReadWrite(database)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.InitializeRoot(context.Background(), "Ada", "Lovelace", "ada@example.test", "correct horse battery"); err != nil {
		t.Fatal(err)
	}
	initialized, err = RootInitialized(context.Background(), database)
	if err != nil || !initialized {
		t.Fatalf("initialized database initialized=%t err=%v", initialized, err)
	}
	root, found, err := store.FindRoot(context.Background())
	if err != nil || !found || root.Login != "root" {
		t.Fatalf("root=%#v found=%t err=%v", root, found, err)
	}
	if err = store.ResetRootPassword(context.Background(), "another safe password"); err != nil {
		t.Fatal(err)
	}
	updated, _, _ := store.FindRoot(context.Background())
	if updated.AuthVersion != root.AuthVersion+1 {
		t.Fatalf("auth version=%d", updated.AuthVersion)
	}
	if _, err = store.database.Exec(`UPDATE users SET two_factor_required=1,totp_secret='TESTSECRET',totp_enabled_at='2026-08-25T10:00:00Z' WHERE type='root'`); err != nil {
		t.Fatal(err)
	}
	if err = store.DisableRootTwoFactor(context.Background()); err != nil {
		t.Fatal(err)
	}
	disabled, _, _ := store.FindRoot(context.Background())
	if disabled.TwoFactorRequired || disabled.TOTPSecret.Valid || disabled.TOTPEnabledAt.Valid {
		t.Fatalf("root two-factor authentication was not cleared: %#v", disabled)
	}
	if disabled.AuthVersion != updated.AuthVersion+1 {
		t.Fatalf("auth version after two-factor reset=%d", disabled.AuthVersion)
	}
}

func TestDisableRootTwoFactorRequiresInitializedRoot(t *testing.T) {
	directory := t.TempDir()
	database := filepath.Join(directory, "aegisadmin.sqlite")
	if err := os.WriteFile(database, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := OpenReadWrite(database)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err = store.database.Exec(`CREATE TABLE users (type TEXT,two_factor_required INTEGER,totp_secret TEXT,totp_enabled_at TEXT,auth_version INTEGER,updated_at TEXT)`); err != nil {
		t.Fatal(err)
	}
	if err = store.DisableRootTwoFactor(context.Background()); err == nil {
		t.Fatal("expected an error when the root account is absent")
	}
}
