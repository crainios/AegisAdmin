package webauth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/bcrypt"
)

const (
	maxArgonMemory        = 256 * 1024
	maxArgonIterations    = 10
	maxArgonParallelism   = 16
	passwordMinimumLength = 12
	passwordMaximumLength = 128
)

func HashPassword(password string) (string, error) {
	if !utf8.ValidString(password) {
		return "", errors.New("password is not valid UTF-8")
	}
	length := utf8.RuneCountInString(password)
	if length < passwordMinimumLength {
		return "", fmt.Errorf("password must contain at least %d characters", passwordMinimumLength)
	}
	if length > passwordMaximumLength {
		return "", fmt.Errorf("password must not exceed %d characters", passwordMaximumLength)
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	const memory, iterations, parallelism = uint32(64 * 1024), uint32(4), uint8(1)
	hash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, 32)
	return fmt.Sprintf(
		"$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		memory, iterations, parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

func VerifyPassword(password, encoded string) bool {
	if password == "" || len(password) > 1024 || encoded == "" {
		return false
	}
	if strings.HasPrefix(encoded, "$argon2id$") {
		return verifyArgon2ID(password, encoded)
	}
	if strings.HasPrefix(encoded, "$2y$") || strings.HasPrefix(encoded, "$2b$") || strings.HasPrefix(encoded, "$2a$") {
		return bcrypt.CompareHashAndPassword([]byte(encoded), []byte(password)) == nil
	}
	return false
}

func verifyArgon2ID(password, encoded string) bool {
	parameters, salt, expected, err := parseArgon2ID(encoded)
	if err != nil {
		return false
	}
	actual := argon2.IDKey(
		[]byte(password), salt, parameters.iterations, parameters.memory,
		parameters.parallelism, uint32(len(expected)),
	)
	return subtle.ConstantTimeCompare(actual, expected) == 1
}

type argonParameters struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
}

func parseArgon2ID(encoded string) (argonParameters, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return argonParameters{}, nil, nil, errors.New("invalid argon2id format")
	}
	values := map[string]uint64{}
	for _, item := range strings.Split(parts[3], ",") {
		key, value, found := strings.Cut(item, "=")
		if !found || (key != "m" && key != "t" && key != "p") {
			return argonParameters{}, nil, nil, errors.New("invalid argon2id parameters")
		}
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil {
			return argonParameters{}, nil, nil, fmt.Errorf("parse argon2id parameter: %w", err)
		}
		values[key] = parsed
	}
	if len(values) != 3 || values["m"] < 8 || values["m"] > maxArgonMemory ||
		values["t"] < 1 || values["t"] > maxArgonIterations ||
		values["p"] < 1 || values["p"] > maxArgonParallelism {
		return argonParameters{}, nil, nil, errors.New("unsafe argon2id parameters")
	}
	decode := func(value string) ([]byte, error) {
		decoded, err := base64.RawStdEncoding.Strict().DecodeString(value)
		if err != nil {
			return nil, err
		}
		return decoded, nil
	}
	salt, err := decode(parts[4])
	if err != nil || len(salt) < 8 || len(salt) > 64 {
		return argonParameters{}, nil, nil, errors.New("invalid argon2id salt")
	}
	hash, err := decode(parts[5])
	if err != nil || len(hash) < 16 || len(hash) > 64 {
		return argonParameters{}, nil, nil, errors.New("invalid argon2id hash")
	}
	return argonParameters{
		memory: uint32(values["m"]), iterations: uint32(values["t"]), parallelism: uint8(values["p"]),
	}, salt, hash, nil
}
