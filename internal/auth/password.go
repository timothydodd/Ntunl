// Package auth provides password hashing, API token generation, portal sessions,
// and net/http middleware for the host portal and tunnel handshake.
package auth

import "golang.org/x/crypto/bcrypt"

const bcryptCost = 12

// HashPassword returns a bcrypt hash of the plaintext password.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	return string(b), err
}

// CheckPassword reports whether password matches the stored bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}
