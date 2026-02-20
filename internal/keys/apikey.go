package keys

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

func NewAPIKey() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func HashAPIKey(pepper, apiKey string) string {
	sum := sha256.Sum256([]byte(pepper + ":" + apiKey))
	return hex.EncodeToString(sum[:])
}
