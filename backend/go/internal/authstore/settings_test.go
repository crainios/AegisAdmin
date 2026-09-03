package authstore

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSMTPSettingsEncryptAndPreservePassword(t *testing.T) {
	database, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err = database.Exec(`CREATE TABLE application_settings(setting_key TEXT PRIMARY KEY,setting_value TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	store := &Store{database: database}
	if err = store.SetSecretKey([]byte("01234567890123456789012345678901")); err != nil {
		t.Fatal(err)
	}
	password := "highly private password"
	wanted := SMTPSettings{Host: "smtp.example.com", Username: "mailer", Port: 465, Security: "tls"}
	if err = store.UpdateSMTPSettings(context.Background(), wanted, &password); err != nil {
		t.Fatal(err)
	}
	var persisted string
	if err = database.QueryRow(`SELECT setting_value FROM application_settings WHERE setting_key='smtp.password'`).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(persisted, password) || !strings.HasPrefix(persisted, encryptedSecretPrefix) {
		t.Fatalf("password is not safely encrypted: %q", persisted)
	}
	got, err := store.SMTPSettings(context.Background())
	if err != nil || !got.PasswordConfigured || got.Host != wanted.Host || got.Port != wanted.Port || got.Security != wanted.Security {
		t.Fatalf("SMTPSettings() = %#v, %v", got, err)
	}
	if clear, err := store.SMTPPassword(context.Background()); err != nil || clear != password {
		t.Fatalf("SMTPPassword() = %q, %v", clear, err)
	}
	if err = store.UpdateSMTPSettings(context.Background(), SMTPSettings{Host: "new.example.com", Port: 587, Security: "starttls"}, nil); err != nil {
		t.Fatal(err)
	}
	if clear, err := store.SMTPPassword(context.Background()); err != nil || clear != password {
		t.Fatalf("preserved password = %q, %v", clear, err)
	}
	empty := ""
	if err = store.UpdateSMTPSettings(context.Background(), wanted, &empty); err != nil {
		t.Fatal(err)
	}
	if clear, err := store.SMTPPassword(context.Background()); err != nil || clear != "" {
		t.Fatalf("cleared password = %q, %v", clear, err)
	}
}
