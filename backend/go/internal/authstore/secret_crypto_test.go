package authstore

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSecretKeyAndAuthenticatedEncryption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "settings.key")
	key, err := LoadOrCreateSecretKey(path)
	if err != nil || len(key) != 32 {
		t.Fatalf("LoadOrCreateSecretKey() = %d bytes, %v", len(key), err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("secret key mode = %v, %v", info.Mode().Perm(), err)
	}
	reloaded, err := LoadOrCreateSecretKey(path)
	if err != nil || !bytes.Equal(key, reloaded) {
		t.Fatalf("reloaded key differs: %v", err)
	}
	store := &Store{}
	if err = store.SetSecretKey(key); err != nil {
		t.Fatal(err)
	}
	encrypted, err := store.encryptSecret("smtp-password")
	if err != nil || strings.Contains(encrypted, "smtp-password") {
		t.Fatalf("encrypted secret = %q, %v", encrypted, err)
	}
	plain, err := store.decryptSecret(encrypted)
	if err != nil || plain != "smtp-password" {
		t.Fatalf("decrypted secret = %q, %v", plain, err)
	}
	other := &Store{}
	_ = other.SetSecretKey(bytes.Repeat([]byte{42}, 32))
	if _, err = other.decryptSecret(encrypted); err == nil {
		t.Fatal("decryption with another key unexpectedly succeeded")
	}
}

func TestSecretKeyRejectsSymlink(t *testing.T) {
	directory := t.TempDir()
	target := filepath.Join(directory, "target")
	if err := os.WriteFile(target, bytes.Repeat([]byte{1}, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadOrCreateSecretKey(link); err == nil {
		t.Fatal("symbolic key unexpectedly accepted")
	}
}
