package webauth

import (
	"encoding/base64"
	"fmt"
	"testing"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

func TestVerifyPasswordSupportsPHPHashes(t *testing.T) {
	password := "Correct horse battery staple"
	salt := []byte("0123456789abcdef")
	hash := argon2.IDKey([]byte(password), salt, 2, 32*1024, 1, 32)
	encoded := fmt.Sprintf(
		"$argon2id$v=19$m=32768,t=2,p=1$%s$%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	if !VerifyPassword(password, encoded) || VerifyPassword("incorrect", encoded) {
		t.Fatal("argon2id verification failed")
	}

	bcryptHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(password, string(bcryptHash)) || VerifyPassword("incorrect", string(bcryptHash)) {
		t.Fatal("bcrypt verification failed")
	}
}

func TestVerifyPasswordRejectsUnsafeOrUnknownHashes(t *testing.T) {
	for _, encoded := range []string{
		"", "$unknown$value", "$argon2id$v=19$m=999999,t=2,p=1$c2FsdHNhbHQ$AAAAAAAAAAAAAAAAAAAAAA",
	} {
		if VerifyPassword("password", encoded) {
			t.Fatalf("unsafe hash accepted: %q", encoded)
		}
	}
}

func TestHashPasswordUsesCompatibleArgon2ID(t *testing.T) {
	password := "Phrase de passe sûre"
	first, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	second, err := HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !VerifyPassword(password, first) || VerifyPassword("mot de passe incorrect", first) {
		t.Fatal("generated Argon2id hash is invalid or deterministic")
	}
}

func TestHashPasswordEnforcesCharacterPolicy(t *testing.T) {
	for _, password := range []string{"trop court", string(make([]byte, 129)), string([]byte{0xff, 0xfe})} {
		if _, err := HashPassword(password); err == nil {
			t.Fatalf("invalid password accepted: %q", password)
		}
	}
	if _, err := HashPassword("éééééééééééé"); err != nil {
		t.Fatalf("twelve Unicode characters rejected: %v", err)
	}
}
