package authstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func TestUserThemeDefaultsAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "aegisadmin.sqlite")
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = database.Exec(`
CREATE TABLE users(id INTEGER PRIMARY KEY);
INSERT INTO users(id) VALUES(1);
CREATE TABLE user_preferences(
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    theme TEXT NOT NULL CHECK(theme IN ('dark','light','bootstrap','neon')),
	language TEXT NOT NULL DEFAULT 'fr' CHECK(language IN ('fr','en')),
    updated_at TEXT NOT NULL
);`); err != nil {
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
	theme, err := store.ThemeForUser(context.Background(), 1)
	if err != nil || theme != "dark" {
		t.Fatalf("default theme=%q err=%v", theme, err)
	}
	if err = store.SetTheme(context.Background(), 1, "neon"); err != nil {
		t.Fatal(err)
	}
	theme, err = store.ThemeForUser(context.Background(), 1)
	if err != nil || theme != "neon" {
		t.Fatalf("stored theme=%q err=%v", theme, err)
	}
	if err = store.SetTheme(context.Background(), 1, "unknown"); err == nil {
		t.Fatal("invalid theme was accepted")
	}
	language, err := store.LanguageForUser(context.Background(), 1)
	if err != nil || language != "fr" {
		t.Fatalf("default language=%q err=%v", language, err)
	}
	if err = store.SetLanguage(context.Background(), 1, "en"); err != nil {
		t.Fatal(err)
	}
	language, err = store.LanguageForUser(context.Background(), 1)
	if err != nil || language != "en" {
		t.Fatalf("stored language=%q err=%v", language, err)
	}
	if err = store.SetLanguage(context.Background(), 1, "de"); err == nil {
		t.Fatal("invalid language was accepted")
	}
}
