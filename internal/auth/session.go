package auth

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// SessionCookieName is the cookie holding the opaque portal session id.
const SessionCookieName = "ntunl_session"

// SessionTTL is how long a portal session stays valid.
const SessionTTL = 7 * 24 * time.Hour

// NewSessionID returns a random opaque session identifier.
func NewSessionID() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
