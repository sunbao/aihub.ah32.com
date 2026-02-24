package agenthome

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/cyberphone/json-canonicalization/go/src/webpki.org/jsoncanonicalizer"
)

var ErrMissingEncryptionKey = errors.New("missing encryption key")

func CanonicalJSON(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return jsoncanonicalizer.Transform(raw)
}

func CanonicalizeJSONBytes(raw []byte) ([]byte, error) {
	return jsoncanonicalizer.Transform(raw)
}

func SignEd25519Base64(privateKey ed25519.PrivateKey, msg []byte) (string, error) {
	if len(privateKey) != ed25519.PrivateKeySize {
		return "", fmt.Errorf("invalid ed25519 private key length: %d", len(privateKey))
	}
	sig := ed25519.Sign(privateKey, msg)
	return base64.StdEncoding.EncodeToString(sig), nil
}

func VerifyEd25519Base64(publicKey ed25519.PublicKey, msg []byte, sigB64 string) (bool, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return false, fmt.Errorf("invalid ed25519 public key length: %d", len(publicKey))
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigB64))
	if err != nil {
		return false, err
	}
	return ed25519.Verify(publicKey, msg, sig), nil
}

func ParseEd25519PublicKey(s string) (ed25519.PublicKey, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty public key")
	}
	if strings.HasPrefix(strings.ToLower(s), "ed25519:") {
		s = strings.TrimSpace(s[len("ed25519:"):])
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(b) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("invalid ed25519 public key length: %d", len(b))
	}
	return ed25519.PublicKey(b), nil
}

func ParseEd25519PrivateKey(s string) (ed25519.PrivateKey, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, errors.New("empty private key")
	}
	if strings.HasPrefix(strings.ToLower(s), "ed25519:") {
		s = strings.TrimSpace(s[len("ed25519:"):])
	}
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	if len(b) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid ed25519 private key length: %d", len(b))
	}
	return ed25519.PrivateKey(b), nil
}

func GenerateEd25519Keypair() (publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey, err error) {
	publicKey, privateKey, err = ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	return publicKey, privateKey, nil
}

func EncryptForDB(encryptionKey string, plaintext []byte) ([]byte, error) {
	encryptionKey = strings.TrimSpace(encryptionKey)
	if encryptionKey == "" {
		return nil, ErrMissingEncryptionKey
	}
	k := sha256.Sum256([]byte(encryptionKey))

	block, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	out := gcm.Seal(nil, nonce, plaintext, nil)
	// DB blob format: nonce || ciphertext
	return append(nonce, out...), nil
}

func DecryptFromDB(encryptionKey string, blob []byte) ([]byte, error) {
	encryptionKey = strings.TrimSpace(encryptionKey)
	if encryptionKey == "" {
		return nil, ErrMissingEncryptionKey
	}
	k := sha256.Sum256([]byte(encryptionKey))

	block, err := aes.NewCipher(k[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	nonce := blob[:gcm.NonceSize()]
	ciphertext := blob[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}
	return plaintext, nil
}

func NewRandomChallenge() (string, error) {
	b := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
