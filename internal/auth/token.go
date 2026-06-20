package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// GenerateToken returns a new random API token (plaintext, shown to the user
// once) alongside its hash for storage.
func GenerateToken() (plaintext, hash string, err error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", "", err
	}
	plaintext = hex.EncodeToString(buf)
	return plaintext, HashToken(plaintext), nil
}

// HashToken returns the storage hash for a plaintext token. Tokens are
// high-entropy random values, so a fast SHA-256 is sufficient (no salt needed).
func HashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
