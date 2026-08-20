package platform

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v3/jwa"
	"github.com/lestrrat-go/jwx/v3/jwt"
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

// TokenManager issues and verifies HS256 JSON Web Tokens.
type TokenManager struct{ key []byte }

// NewTokenManager constructs a token manager from a sufficiently long secret.
func NewTokenManager(secret string) (*TokenManager, error) {
	if len(strings.TrimSpace(secret)) < 32 {
		return nil, errors.New("token secret must be at least 32 characters")
	}
	return &TokenManager{key: []byte(secret)}, nil
}

// Issue signs claims with the supplied lifetime and returns the token and final
// claims, including generated timestamps and token ID.
func (m *TokenManager) Issue(claims TokenClaims, ttl time.Duration) (string, TokenClaims, error) {
	now := time.Now().UTC()
	claims.IssuedAt, claims.ExpiresAt = now.Unix(), now.Add(ttl).Unix()
	if claims.TokenID == "" {
		claims.TokenID = NewID("tok")
	}
	// The registered names carry the fields JWT already defines one for, so a token
	// stays legible to any standard tool; everything else is a private claim.
	builder := jwt.NewBuilder().
		Subject(claims.SubjectID).
		JwtID(claims.TokenID).
		IssuedAt(now).
		Expiration(now.Add(ttl)).
		Claim("subject_type", claims.SubjectType).
		Claim("token_type", claims.TokenType)
	for name, value := range map[string]string{
		"credential_id": claims.CredentialID,
		"email":         claims.Email,
		"role":          claims.Role,
		"scope":         claims.Scopes,
	} {
		if value != "" {
			builder = builder.Claim(name, value)
		}
	}
	if len(claims.Extra) > 0 {
		builder = builder.Claim("extra", claims.Extra)
	}
	token, err := builder.Build()
	if err != nil {
		return "", claims, err
	}
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.HS256(), m.key))
	if err != nil {
		return "", claims, err
	}
	return string(signed), claims, nil
}

// Verify authenticates token and rejects a malformed, mis-signed, or expired one.
// Parse validates the registered lifetime claims by default.
func (m *TokenManager) Verify(token string) (*TokenClaims, error) {
	verified, err := jwt.Parse([]byte(token), jwt.WithKey(jwa.HS256(), m.key))
	if err != nil {
		return nil, err
	}
	return claimsFrom(verified), nil
}

// claimsFrom projects a verified JWT back onto TokenClaims. A claim the token does not
// carry is left at its zero value: presence is the caller's business, and every field
// that authorizes anything is re-read from the database anyway.
func claimsFrom(token jwt.Token) *TokenClaims {
	claims := TokenClaims{}
	claims.SubjectID, _ = token.Subject()
	claims.TokenID, _ = token.JwtID()
	if issued, ok := token.IssuedAt(); ok {
		claims.IssuedAt = issued.Unix()
	}
	if expires, ok := token.Expiration(); ok {
		claims.ExpiresAt = expires.Unix()
	}
	for name, field := range map[string]*string{
		"subject_type":  &claims.SubjectType,
		"credential_id": &claims.CredentialID,
		"email":         &claims.Email,
		"role":          &claims.Role,
		"scope":         &claims.Scopes,
		"token_type":    &claims.TokenType,
	} {
		_ = token.Get(name, field)
	}
	// A private claim decodes as map[string]any, so it needs narrowing rather than a
	// direct assignment into the map[string]string field.
	var extra map[string]any
	if token.Get("extra", &extra) == nil && len(extra) > 0 {
		claims.Extra = make(map[string]string, len(extra))
		for name, value := range extra {
			if text, ok := value.(string); ok {
				claims.Extra[name] = text
			}
		}
	}
	return &claims
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
