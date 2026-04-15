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

func TestHashAdminToken(t *testing.T) {
	hash, err := auth.HashToken("admin-secret")
	if err != nil {
		t.Fatalf("HashToken() error: %v", err)
	}
	if !auth.VerifyToken("admin-secret", hash) {
		t.Error("VerifyToken() failed for admin token")
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
