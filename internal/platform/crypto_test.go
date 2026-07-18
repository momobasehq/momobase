package platform

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func testEncryptionKey() string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32))
}

func TestEncryptorRoundTripAndAuthentication(t *testing.T) {
	encryptor, err := NewEncryptor(testEncryptionKey())
	if err != nil {
		t.Fatalf("NewEncryptor() error = %v", err)
	}
	plain := []byte(`{"client_secret":"sensitive"}`)
	first, err := encryptor.Encrypt(plain)
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	second, err := encryptor.Encrypt(plain)
	if err != nil {
		t.Fatalf("second Encrypt() error = %v", err)
	}
	if first == second {
		t.Fatal("Encrypt() reused a nonce for identical plaintext")
	}
	decrypted, err := encryptor.Decrypt(first)
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if !bytes.Equal(decrypted, plain) {
		t.Fatalf("Decrypt() = %q", decrypted)
	}

	raw, err := base64.StdEncoding.DecodeString(first)
	if err != nil {
		t.Fatalf("decode ciphertext: %v", err)
	}
	raw[len(raw)-1] ^= 0x01
	if _, err := encryptor.Decrypt(base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Fatal("Decrypt() accepted tampered ciphertext")
	}
}

func TestEncryptorRejectsInvalidInputs(t *testing.T) {
	if _, err := NewEncryptor("not-base64"); err == nil {
		t.Fatal("NewEncryptor() accepted invalid base64")
	}
	shortKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 31))
	if _, err := NewEncryptor(shortKey); err == nil {
		t.Fatal("NewEncryptor() accepted a non-256-bit key")
	}

	encryptor, err := NewEncryptor(testEncryptionKey())
	if err != nil {
		t.Fatalf("NewEncryptor() error = %v", err)
	}
	if _, err := encryptor.Decrypt("not-base64"); err == nil {
		t.Fatal("Decrypt() accepted invalid base64")
	}
	if _, err := encryptor.Decrypt(base64.StdEncoding.EncodeToString([]byte("short"))); err == nil {
		t.Fatal("Decrypt() accepted a short ciphertext")
	}
}

func TestTokenManagerIssueAndVerify(t *testing.T) {
	manager, err := NewTokenManager(strings.Repeat("s", 32))
	if err != nil {
		t.Fatalf("NewTokenManager() error = %v", err)
	}
	token, issued, err := manager.Issue(TokenClaims{
		SubjectType: "app",
		SubjectID:   "app-1",
		TokenType:   "access",
		Scopes:      "transactions:read",
		Extra:       map[string]string{"client_id": "client-1"},
	}, time.Minute)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if issued.TokenID == "" || issued.IssuedAt == 0 || issued.ExpiresAt <= issued.IssuedAt {
		t.Fatalf("Issue() claims = %+v", issued)
	}
	verified, err := manager.Verify(token)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if verified.SubjectID != "app-1" || verified.Extra["client_id"] != "client-1" {
		t.Fatalf("Verify() claims = %+v", verified)
	}

	other, err := NewTokenManager(strings.Repeat("x", 32))
	if err != nil {
		t.Fatalf("NewTokenManager() error = %v", err)
	}
	if _, err := other.Verify(token); err == nil {
		t.Fatal("Verify() accepted a token signed with another secret")
	}
	if _, err := manager.Verify(token + "tampered"); err == nil {
		t.Fatal("Verify() accepted a tampered token")
	}
}

func TestTokenManagerRejectsInvalidAndExpiredTokens(t *testing.T) {
	if _, err := NewTokenManager("short"); err == nil {
		t.Fatal("NewTokenManager() accepted a short secret")
	}
	manager, err := NewTokenManager(strings.Repeat("s", 32))
	if err != nil {
		t.Fatalf("NewTokenManager() error = %v", err)
	}
	if _, err := manager.Verify("missing-separator"); err == nil {
		t.Fatal("Verify() accepted a malformed token")
	}

	invalidPayload := "%%%"
	if _, err := manager.Verify(invalidPayload + "." + manager.sign(invalidPayload)); err == nil {
		t.Fatal("Verify() accepted an invalid payload encoding")
	}
	invalidClaims := base64.RawURLEncoding.EncodeToString([]byte("["))
	if _, err := manager.Verify(invalidClaims + "." + manager.sign(invalidClaims)); err == nil {
		t.Fatal("Verify() accepted invalid claims JSON")
	}

	expired, _, err := manager.Issue(TokenClaims{SubjectID: "expired"}, -time.Second)
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if _, err := manager.Verify(expired); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("Verify(expired) error = %v", err)
	}
}

func TestIdentityPasswordAndHashHelpers(t *testing.T) {
	id := NewID("app")
	if !strings.HasPrefix(id, "app_") {
		t.Fatalf("NewID() = %q", id)
	}
	token, err := SecureRandomToken("secret", 8)
	if err != nil {
		t.Fatalf("SecureRandomToken() error = %v", err)
	}
	if !strings.HasPrefix(token, "secret_") || len(strings.TrimPrefix(token, "secret_")) < 40 {
		t.Fatalf("SecureRandomToken() = %q", token)
	}
	if got := SHA256Hex("momobase"); len(got) != 64 || got == SHA256Hex("other") {
		t.Fatalf("SHA256Hex() = %q", got)
	}
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword() error = %v", err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") || VerifyPassword(hash, "wrong") {
		t.Fatal("VerifyPassword() returned an incorrect result")
	}
}
