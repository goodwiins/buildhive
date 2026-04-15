package auth

import (
	"crypto/rand"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// GenerateToken creates a cryptographically random token and its bcrypt hash.
// The plaintext token is returned for one-time display to the user.
func GenerateToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	token = hex.EncodeToString(b)
	hash, err = hashRaw(token)
	return
}

// HashToken hashes an existing plaintext token (e.g. the admin token from env).
func HashToken(token string) (string, error) {
	return hashRaw(token)
}

// VerifyToken checks a plaintext token against a bcrypt hash.
// Returns false if token or hash is empty.
func VerifyToken(token, hash string) bool {
	if token == "" || hash == "" {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(token))
	return err == nil
}

func hashRaw(token string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(token), bcryptCost)
	return string(h), err
}
