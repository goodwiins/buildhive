package auth_test

import (
	"testing"

	"github.com/buildhive/buildhive/internal/auth"
)

func TestGenerateAndVerify(t *testing.T) {
	token, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	if len(token) != 64 {
		t.Errorf("expected 64-char hex token, got %d chars", len(token))
	}
	if !auth.VerifyToken(token, hash) {
		t.Error("VerifyToken() returned false for valid token")
	}
}

func TestVerifyInvalidToken(t *testing.T) {
	_, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	if auth.VerifyToken("wrongtoken", hash) {
		t.Error("VerifyToken() returned true for wrong token")
	}
}

func TestHashToken(t *testing.T) {
	hash := auth.HashToken("admin-secret")
	if len(hash) != 64 {
		t.Errorf("expected 64-char SHA-256 hex, got %d", len(hash))
	}
	if !auth.VerifyToken("admin-secret", hash) {
		t.Error("VerifyToken() failed for known hash")
	}
}

func TestVerifyEmptyToken(t *testing.T) {
	_, hash, err := auth.GenerateToken()
	if err != nil {
		t.Fatalf("GenerateToken() error: %v", err)
	}
	if auth.VerifyToken("", hash) {
		t.Error("VerifyToken() returned true for empty token")
	}
	token2, _, err2 := auth.GenerateToken()
	if err2 != nil {
		t.Fatalf("GenerateToken() error: %v", err2)
	}
	if auth.VerifyToken(token2, "") {
		t.Error("VerifyToken() returned true for empty hash")
	}
}
