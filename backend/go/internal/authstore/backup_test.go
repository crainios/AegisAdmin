package authstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestBackupAndRestoreDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aegisadmin.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = database.Exec(`
CREATE TABLE schema_migrations(name TEXT, checksum TEXT);
CREATE TABLE navigation_categories(id INTEGER);
CREATE TABLE navigation_modules(id INTEGER);
CREATE TABLE user_module_permissions(user_id INTEGER);
CREATE TABLE application_settings(setting_key TEXT);
CREATE TABLE user_access_log(id INTEGER);
CREATE TABLE users(id INTEGER PRIMARY KEY,login TEXT,password_hash TEXT,type TEXT,status TEXT,must_change_password INTEGER,auth_version INTEGER,two_factor_required INTEGER,totp_secret TEXT,totp_enabled_at TEXT);
INSERT INTO users VALUES(1,'root','hash','root','active',0,3,0,NULL,NULL);`)
	if err != nil {
		t.Fatal(err)
	}
	database.Close()
	store, err := OpenReadWrite(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	backup, err := store.BackupDatabase(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.database.Exec(`UPDATE users SET login='changed'`); err != nil {
		t.Fatal(err)
	}
	if err = store.RestoreDatabase(context.Background(), backup); err != nil {
		t.Fatal(err)
	}
	user, found, err := store.FindRoot(context.Background())
	if err != nil || !found || user.Login != "root" || user.AuthVersion != 4 {
		t.Fatalf("restored user=%#v found=%t err=%v", user, found, err)
	}
}
