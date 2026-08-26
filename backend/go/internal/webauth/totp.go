package webauth

import (
	"crypto/hmac"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"strings"
	"time"
)

// TOTPVerifier validates the SHA-1, six-digit, 30-second TOTP profile used by
// compatible authenticator applications.
type TOTPVerifier struct {
	now func() time.Time
}

func NewTOTPVerifier() *TOTPVerifier {
	return &TOTPVerifier{now: time.Now}
}

func (v *TOTPVerifier) Verify(secret, code string) bool {
	code = strings.Join(strings.Fields(code), "")
	if len(code) != 6 {
		return false
	}
	for _, character := range code {
		if character < '0' || character > '9' {
			return false
		}
	}
	key, err := decodeTOTPSecret(secret)
	if err != nil {
		return false
	}
	counter := v.now().Unix() / 30
	for offset := int64(-1); offset <= 1; offset++ {
		expected := totpCode(key, uint64(counter+offset))
		if subtle.ConstantTimeCompare([]byte(expected), []byte(code)) == 1 {
			return true
		}
	}
	return false
}

func decodeTOTPSecret(secret string) ([]byte, error) {
	secret = strings.ToUpper(strings.Join(strings.Fields(secret), ""))
	if secret == "" || len(secret) > 256 {
		return nil, errors.New("invalid TOTP secret")
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
}

func totpCode(key []byte, counter uint64) string {
	value := make([]byte, 8)
	binary.BigEndian.PutUint64(value, counter)
	digest := hmac.New(sha1.New, key)
	_, _ = digest.Write(value)
	sum := digest.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	number := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])
	number %= 1_000_000
	return string([]byte{
		byte(number/100_000) + '0', byte(number/10_000%10) + '0',
		byte(number/1_000%10) + '0', byte(number/100%10) + '0',
		byte(number/10%10) + '0', byte(number%10) + '0',
	})
}
