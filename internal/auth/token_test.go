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
	if len(token) < 32 {
		t.Errorf("token too short: %d chars", len(token))
	}
	if !auth.VerifyToken(token, hash) {
		t.Error("VerifyToken() returned false for valid token")
	}
}

func TestVerifyInvalidToken(t *testing.T) {
	_, hash, _ := auth.GenerateToken()
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
	_, hash, _ := auth.GenerateToken()
	if auth.VerifyToken("", hash) {
		t.Error("VerifyToken() returned true for empty token")
	}
}
