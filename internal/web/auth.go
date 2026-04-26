package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	SessionName     = "easywall"
	SessionLifetime = 600 // seconds
	SessionUserKey  = "user"
)

type argon2Params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

var defaultArgon2Params = argon2Params{
	memory:      64 * 1024,
	iterations:  3,
	parallelism: 4,
	saltLength:  16,
	keyLength:   32,
}

// HashPassword returns an argon2id encoded hash of the given password.
func HashPassword(password string) (string, error) {
	p := defaultArgon2Params
	salt := make([]byte, p.saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}

	hash := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, p.keyLength)

	b64salt := base64.RawStdEncoding.EncodeToString(salt)
	b64hash := base64.RawStdEncoding.EncodeToString(hash)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.iterations, p.parallelism, b64salt, b64hash), nil
}

// VerifyPassword returns true if password matches the argon2id encoded hash.
func VerifyPassword(password, encodedHash string) bool {
	p, salt, hash, err := decodeArgon2Hash(encodedHash)
	if err != nil {
		return false
	}
	other := argon2.IDKey([]byte(password), salt, p.iterations, p.memory, p.parallelism, p.keyLength)
	return subtle.ConstantTimeCompare(hash, other) == 1
}

// decodeArgon2Hash parses a $argon2id$... encoded hash.
func decodeArgon2Hash(encoded string) (argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	// expected: ["", "argon2id", "v=19", "m=65536,t=3,p=4", "<salt>", "<hash>"]
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argon2Params{}, nil, nil, fmt.Errorf("invalid argon2id hash format")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("parse version: %w", err)
	}
	if version != argon2.Version {
		return argon2Params{}, nil, nil, fmt.Errorf("unsupported argon2 version %d", version)
	}

	var p argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.iterations, &p.parallelism); err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("parse params: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("decode salt: %w", err)
	}

	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("decode hash: %w", err)
	}

	p.saltLength = uint32(len(salt))
	p.keyLength = uint32(len(hash))
	return p, salt, hash, nil
}
