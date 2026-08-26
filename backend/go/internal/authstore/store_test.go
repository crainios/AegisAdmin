package authstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestReadOnlyStoreChecksSchemaAndFindsUser(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aegisadmin.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`
CREATE TABLE users (
    id INTEGER PRIMARY KEY, login TEXT COLLATE NOCASE, password_hash TEXT,
    type TEXT, status TEXT, must_change_password INTEGER, auth_version INTEGER,
    two_factor_required INTEGER, totp_secret TEXT, totp_enabled_at TEXT
);
INSERT INTO users VALUES (1, 'Root', 'hash', 'root', 'active', 0, 2, 1, 'secret', NULL);
`)
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	user, found, err := store.FindByLogin(context.Background(), "root")
	if err != nil || !found {
		t.Fatalf("found = %t, err = %v", found, err)
	}
	if user.ID != 1 || user.Login != "Root" || !user.TwoFactorRequired || user.TOTPEnabledAt.Valid {
		t.Fatalf("unexpected user: %#v", user)
	}
}

func TestStoreRejectsIncompleteSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aegisadmin.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, login TEXT)`); err != nil {
		t.Fatal(err)
	}
	database.Close()

	store, err := OpenReadOnly(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Check(context.Background()); err == nil {
		t.Fatal("incomplete schema was accepted")
	}
}

func TestReadWriteStorePreparesAndEnablesTOTP(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aegisadmin.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`
CREATE TABLE users (
    id INTEGER PRIMARY KEY, login TEXT COLLATE NOCASE, password_hash TEXT,
    type TEXT, status TEXT, must_change_password INTEGER, auth_version INTEGER,
    two_factor_required INTEGER, totp_secret TEXT, totp_enabled_at TEXT,
    updated_at TEXT, password_changed_at TEXT
);
INSERT INTO users VALUES (1, 'admin', 'hash', 'user', 'active', 0, 4, 1, NULL, NULL, 'old', 'old');
`)
	if err != nil {
		t.Fatal(err)
	}
	if err = database.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	prepared, err := store.PrepareTOTP(context.Background(), 1, 4, "FIRSTSECRET")
	if err != nil || !prepared.TOTPSecret.Valid || prepared.TOTPSecret.String != "FIRSTSECRET" {
		t.Fatalf("prepared = %#v, err = %v", prepared, err)
	}
	preparedAgain, err := store.PrepareTOTP(context.Background(), 1, 4, "REPLACEMENT")
	if err != nil || preparedAgain.TOTPSecret.String != "FIRSTSECRET" {
		t.Fatalf("prepared secret was replaced: %#v, err = %v", preparedAgain, err)
	}
	enabled, err := store.EnableTOTP(context.Background(), 1, 4, "FIRSTSECRET")
	if err != nil || !enabled.TOTPEnabledAt.Valid || enabled.AuthVersion != 5 {
		t.Fatalf("enabled = %#v, err = %v", enabled, err)
	}
	if _, err = store.EnableTOTP(context.Background(), 1, 4, "FIRSTSECRET"); err == nil {
		t.Fatal("stale activation unexpectedly succeeded")
	}
}

func TestReadWriteStoreChangesPasswordConditionally(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aegisadmin.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`
CREATE TABLE users (
    id INTEGER PRIMARY KEY, login TEXT, password_hash TEXT, type TEXT,
    status TEXT, must_change_password INTEGER, auth_version INTEGER,
    two_factor_required INTEGER, totp_secret TEXT, totp_enabled_at TEXT,
    updated_at TEXT, password_changed_at TEXT
);
INSERT INTO users VALUES (1, 'admin', 'old-hash', 'user', 'active', 1, 9, 0, NULL, NULL, 'old', 'old');
`)
	if err != nil {
		t.Fatal(err)
	}
	database.Close()
	store, err := OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	updated, err := store.ChangePassword(context.Background(), 1, 9, "old-hash", "new-hash")
	if err != nil || updated.PasswordHash != "new-hash" || updated.MustChangePassword || updated.AuthVersion != 10 {
		t.Fatalf("updated = %#v, err = %v", updated, err)
	}
	if _, err = store.ChangePassword(context.Background(), 1, 9, "old-hash", "other-hash"); err == nil {
		t.Fatal("stale password update unexpectedly succeeded")
	}
}

func TestMenuHonorsRootAndUserPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aegisadmin.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`
CREATE TABLE navigation_categories (id INTEGER PRIMARY KEY, name TEXT, position INTEGER);
CREATE TABLE navigation_modules (
    id INTEGER PRIMARY KEY, module_key TEXT, name TEXT, route TEXT, icon TEXT,
    category_id INTEGER, position INTEGER, is_enabled INTEGER, access_policy TEXT
);
CREATE TABLE user_module_permissions (user_id INTEGER, module_id INTEGER, permission_level TEXT);
INSERT INTO navigation_categories VALUES (1, 'Supervision', 0), (2, 'Information', 1);
INSERT INTO navigation_modules VALUES
    (1, 'dashboard', 'Dashboard', '/dashboard', 'D', 1, 0, 1, 'view'),
    (2, 'storage', 'Stockage', '/storage', 'S', 1, 1, 1, 'view'),
    (3, 'setting', 'Paramètres', '/setting', 'P', 1, 2, 1, 'root'),
    (4, 'hidden', 'Masqué', '/hidden', 'H', 1, 3, 0, 'view'),
    (5, 'about', 'À propos de…', '/about', 'A', 2, 0, 1, 'view');
INSERT INTO user_module_permissions VALUES (8, 1, 'view'), (8, 2, 'action');
`)
	if err != nil {
		t.Fatal(err)
	}
	database.Close()
	store, err := OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ordinary, err := store.Menu(context.Background(), User{ID: 8, Type: "user"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ordinary) != 2 || len(ordinary[0].Modules) != 2 ||
		ordinary[0].Modules[1].Key != "storage" || ordinary[0].Modules[1].PermissionLevel != "action" ||
		ordinary[1].Modules[0].Key != "about" {
		t.Fatalf("unexpected ordinary menu: %#v", ordinary)
	}
	root, err := store.Menu(context.Background(), User{ID: 1, Type: "root"})
	if err != nil {
		t.Fatal(err)
	}
	if len(root) != 2 || len(root[0].Modules) != 3 || root[0].Modules[2].Key != "setting" ||
		root[0].Modules[2].PermissionLevel != "modify" {
		t.Fatalf("unexpected root menu: %#v", root)
	}
	level, granted, err := store.PermissionForModule(context.Background(), User{ID: 8, Type: "user"}, "storage")
	if err != nil || !granted || level != "action" {
		t.Fatalf("unexpected storage permission: level=%q granted=%t err=%v", level, granted, err)
	}
	if _, granted, err = store.PermissionForModule(context.Background(), User{ID: 8, Type: "user"}, "setting"); err != nil || granted {
		t.Fatalf("root-only module granted to user: granted=%t err=%v", granted, err)
	}
	level, granted, err = store.PermissionForModule(context.Background(), User{ID: 1, Type: "root"}, "setting")
	if err != nil || !granted || level != "modify" {
		t.Fatalf("root permission missing: level=%q granted=%t err=%v", level, granted, err)
	}
}
