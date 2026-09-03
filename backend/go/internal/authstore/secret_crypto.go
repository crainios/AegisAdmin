package authstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const encryptedSecretPrefix = "aegisadmin:v1:"

func LoadOrCreateSecretKey(path string) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("secret key path must be absolute")
	}
	info, err := os.Lstat(path)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
			return nil, errors.New("secret key must be a private regular non-symlink file")
		}
		key, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil, fmt.Errorf("read secret key: %w", readErr)
		}
		if len(key) != 32 {
			return nil, errors.New("secret key has an invalid size")
		}
		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read secret key: %w", err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create secret directory: %w", err)
	}
	key := make([]byte, 32)
	if _, err = io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate secret key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return LoadOrCreateSecretKey(path)
		}
		return nil, fmt.Errorf("create secret key: %w", err)
	}
	if _, err = file.Write(key); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("write secret key: %w", err)
	}
	if err = file.Close(); err != nil {
		return nil, fmt.Errorf("close secret key: %w", err)
	}
	return key, nil
}

func (s *Store) encryptSecret(value string) (string, error) {
	if len(s.secretKey) != 32 {
		return "", errors.New("secret encryption is unavailable")
	}
	block, err := aes.NewCipher(s.secretKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(value), []byte("smtp.password"))
	return encryptedSecretPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

func (s *Store) decryptSecret(value string) (string, error) {
	if len(s.secretKey) != 32 || !strings.HasPrefix(value, encryptedSecretPrefix) {
		return "", errors.New("encrypted secret is invalid")
	}
	payload, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(value, encryptedSecretPrefix))
	if err != nil {
		return "", errors.New("encrypted secret is invalid")
	}
	block, err := aes.NewCipher(s.secretKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(payload) < gcm.NonceSize() {
		return "", errors.New("encrypted secret is invalid")
	}
	plain, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], []byte("smtp.password"))
	if err != nil {
		return "", errors.New("encrypted secret cannot be decrypted")
	}
	return string(plain), nil
}
