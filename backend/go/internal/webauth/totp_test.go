package webauth

import (
	"testing"
	"time"
)

func TestTOTPVerifier(t *testing.T) {
	verifier := NewTOTPVerifier()
	verifier.now = func() time.Time { return time.Unix(59, 0) }
	const secret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	if !verifier.Verify(secret, "287082") {
		t.Fatal("valid TOTP code rejected")
	}
	if !verifier.Verify(secret, " 287 082 ") {
		t.Fatal("spaced TOTP code rejected")
	}
	for _, invalid := range []string{"28708", "2870820", "abcdef", "000000"} {
		if verifier.Verify(secret, invalid) {
			t.Fatalf("invalid TOTP code accepted: %q", invalid)
		}
	}
}

func TestTOTPVerifierAcceptsAdjacentPeriod(t *testing.T) {
	verifier := NewTOTPVerifier()
	verifier.now = func() time.Time { return time.Unix(89, 0) }
	if !verifier.Verify("GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ", "287082") {
		t.Fatal("previous TOTP period rejected")
	}
	if verifier.Verify("invalid!", "287082") {
		t.Fatal("invalid secret accepted")
	}
}
