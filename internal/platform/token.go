package platform

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// TokenClaims contains the signed identity, authorization, and lifetime data in
// a Momobase token.
type TokenClaims struct {
	SubjectType  string            `json:"subject_type"`
	SubjectID    string            `json:"subject_id"`
	CredentialID string            `json:"credential_id,omitempty"`
	Email        string            `json:"email,omitempty"`
	Role         string            `json:"role,omitempty"`
	Scopes       string            `json:"scope,omitempty"`
	TokenType    string            `json:"token_type"`
	TokenID      string            `json:"token_id"`
	ExpiresAt    int64             `json:"exp"`
	IssuedAt     int64             `json:"iat"`
	Extra        map[string]string `json:"extra,omitempty"`
}

// TokenManager issues and verifies HMAC-signed application tokens.
type TokenManager struct{ secret []byte }

// NewTokenManager constructs a token manager from a sufficiently long secret.
func NewTokenManager(secret string) (*TokenManager, error) {
	if len(strings.TrimSpace(secret)) < 32 {
		return nil, errors.New("token secret must be at least 32 characters")
	}
	return &TokenManager{secret: []byte(secret)}, nil
}

// Issue signs claims with the supplied lifetime and returns the token and final
// claims, including generated timestamps and token ID.
func (m *TokenManager) Issue(claims TokenClaims, ttl time.Duration) (string, TokenClaims, error) {
	now := time.Now().UTC()
	claims.IssuedAt, claims.ExpiresAt = now.Unix(), now.Add(ttl).Unix()
	if claims.TokenID == "" {
		claims.TokenID = NewID("tok")
	}
	raw, err := json.Marshal(claims)
	if err != nil {
		return "", claims, err
	}
	payload := base64.RawURLEncoding.EncodeToString(raw)
	return payload + "." + m.sign(payload), claims, nil
}

// Verify authenticates token and rejects malformed or expired claims.
func (m *TokenManager) Verify(token string) (*TokenClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(m.sign(parts[0])), []byte(parts[1])) {
		return nil, errors.New("invalid token")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, errors.New("invalid token payload")
	}
	var claims TokenClaims
	if json.Unmarshal(raw, &claims) != nil {
		return nil, errors.New("invalid token claims")
	}
	if claims.ExpiresAt <= time.Now().UTC().Unix() {
		return nil, errors.New("token expired")
	}
	return &claims, nil
}
func (m *TokenManager) sign(payload string) string {
	mac := hmac.New(sha256.New, m.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// NewID returns a UUID string with an optional prefix.
func NewID(prefix string) string {
	return withPrefix(prefix, uuid.NewString())
}

// SecureRandomToken returns a URL-safe token from cryptographically secure
// random bytes and applies an optional prefix.
func SecureRandomToken(prefix string, size int) (string, error) {
	if size < 16 {
		size = 32
	}
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return withPrefix(prefix, base64.RawURLEncoding.EncodeToString(raw)), nil
}
func withPrefix(prefix, value string) string {
	if prefix != "" {
		return prefix + "_" + value
	}
	return value
}

// SHA256Hex returns the lowercase hexadecimal SHA-256 digest of value.
func SHA256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// HashPassword hashes password with bcrypt's default cost.
func HashPassword(password string) (string, error) {
	raw, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(raw), err
}

// VerifyPassword reports whether password matches a bcrypt hash.
func VerifyPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
