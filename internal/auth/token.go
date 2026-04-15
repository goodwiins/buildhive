package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateToken creates a cryptographically random 32-byte token.
// Returns the plaintext token (for one-time display) and its SHA-256 hash (for DB storage).
func GenerateToken() (token, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	token = hex.EncodeToString(b)
	hash = HashToken(token)
	return
}

// HashToken returns the SHA-256 hex digest of a token.
// Used as the deterministic DB lookup key.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// VerifyToken checks a plaintext token against a SHA-256 hash.
func VerifyToken(token, hash string) bool {
	if token == "" || hash == "" {
		return false
	}
	return HashToken(token) == hash
}
