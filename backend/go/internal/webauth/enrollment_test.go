package webauth

import (
	"net/url"
	"strings"
	"testing"
)

func TestGenerateTOTPSecret(t *testing.T) {
	first, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 || first == second {
		t.Fatalf("unexpected generated secrets: %q %q", first, second)
	}
	if _, err = decodeTOTPSecret(first); err != nil {
		t.Fatalf("generated secret is invalid: %v", err)
	}
}

func TestTOTPProvisioningURI(t *testing.T) {
	value, err := TOTPProvisioningURI(" admin@example.net ", "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(value)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Scheme != "otpauth" || parsed.Host != "totp" ||
		!strings.Contains(parsed.Path, "AegisAdmin:admin@example.net") ||
		parsed.Query().Get("issuer") != "AegisAdmin" || parsed.Query().Get("digits") != "6" {
		t.Fatalf("unexpected provisioning URI: %q", value)
	}
}
