package web

import (
	"strings"
	"testing"

)

func TestHashPassword_ProducesArgon2idFormat(t *testing.T) {
	hash, err := HashPassword("correcthorsebatterystaple")
	if err != nil {
		t.Fatalf("HashPassword error: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$") {
		t.Errorf("expected argon2id prefix, got: %s", hash)
	}
	parts := strings.Split(hash, "$")
	if len(parts) != 6 {
		t.Errorf("expected 6 parts, got %d: %v", len(parts), parts)
	}
}

func TestHashPassword_DifferentSaltsEachCall(t *testing.T) {
	h1, _ := HashPassword("samepassword")
	h2, _ := HashPassword("samepassword")
	if h1 == h2 {
		t.Error("two hashes of same password must differ (different salts)")
	}
}

func TestVerifyPassword_CorrectPassword(t *testing.T) {
	const pw = "s3cr3tP@ssw0rd!"
	hash, err := HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword(pw, hash) {
		t.Error("VerifyPassword returned false for correct password")
	}
}

func TestVerifyPassword_WrongPassword(t *testing.T) {
	hash, _ := HashPassword("correctpassword")
	if VerifyPassword("wrongpassword", hash) {
		t.Error("VerifyPassword returned true for wrong password")
	}
}

func TestVerifyPassword_EmptyPassword(t *testing.T) {
	hash, _ := HashPassword("somepassword")
	if VerifyPassword("", hash) {
		t.Error("VerifyPassword must return false for empty password")
	}
}

func TestVerifyPassword_InvalidHash(t *testing.T) {
	if VerifyPassword("pw", "not-a-valid-hash") {
		t.Error("VerifyPassword must return false for invalid hash")
	}
	if VerifyPassword("pw", "") {
		t.Error("VerifyPassword must return false for empty hash")
	}
}

func TestVerifyPassword_TamperedHash(t *testing.T) {
	hash, _ := HashPassword("mypassword")
	// Replace the hash segment (last $-separated part) with zeros
	parts := strings.Split(hash, "$")
	parts[5] = strings.Repeat("A", len(parts[5]))
	tampered := strings.Join(parts, "$")
	if VerifyPassword("mypassword", tampered) {
		t.Error("VerifyPassword must return false for tampered hash")
	}
}

func TestDecodeArgon2Hash_InvalidFormat(t *testing.T) {
	cases := []string{
		"",
		"$bcrypt$...",
		"$argon2id$v=19$m=65536,t=3,p=4$salt",   // too few parts
		"$argon2id$v=99$m=65536,t=3,p=4$abc$xyz", // wrong version
	}
	for _, c := range cases {
		_, _, _, err := decodeArgon2Hash(c)
		if err == nil {
			t.Errorf("expected error decoding %q", c)
		}
	}
}

func TestHashPassword_Empty(t *testing.T) {
	hash, err := HashPassword("")
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyPassword("", hash) {
		t.Error("empty password hash should verify against empty password")
	}
	if VerifyPassword("x", hash) {
		t.Error("non-empty password must not match empty hash")
	}
}
