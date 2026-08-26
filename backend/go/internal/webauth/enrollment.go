package webauth

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"net/url"
	"strings"
)

func GenerateTOTPSecret() (string, error) {
	value := make([]byte, 20)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(value), nil
}

func TOTPProvisioningURI(login, secret string) (string, error) {
	if _, err := decodeTOTPSecret(secret); err != nil {
		return "", errors.New("invalid TOTP secret")
	}
	label := "AegisAdmin:" + strings.TrimSpace(login)
	query := url.Values{
		"secret": {secret}, "issuer": {"AegisAdmin"}, "algorithm": {"SHA1"},
		"digits": {"6"}, "period": {"30"},
	}
	return "otpauth://totp/" + url.PathEscape(label) + "?" + query.Encode(), nil
}
