package providers

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// RandomRef returns prefix followed by a cryptographically random hexadecimal ID.
// It falls back to a timestamp-based ID when the system random source fails.
func RandomRef(prefix string) string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("%s%d", prefix, time.Now().UnixNano())
	}
	return prefix + hex.EncodeToString(raw[:])
}

// UUID returns a UUID-shaped random reference, or RandomRef's fallback unchanged.
func UUID() string {
	raw := RandomRef("")
	if len(raw) != 32 {
		return raw
	}
	return raw[:8] + "-" + raw[8:12] + "-4" + raw[13:16] + "-a" + raw[17:20] + "-" + raw[20:]
}
