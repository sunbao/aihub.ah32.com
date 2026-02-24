package agenthome

import (
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

type Cert struct {
	Issuer    string `json:"issuer"`
	KeyID     string `json:"key_id"`
	IssuedAt  string `json:"issued_at"`
	ExpiresAt string `json:"expires_at"`
	Alg       string `json:"alg"`
	Signature string `json:"signature"`
}

func NewCert(issuer, keyID, alg string, issuedAt, expiresAt time.Time, signatureB64 string) Cert {
	return Cert{
		Issuer:    issuer,
		KeyID:     keyID,
		IssuedAt:  issuedAt.UTC().Format(time.RFC3339),
		ExpiresAt: expiresAt.UTC().Format(time.RFC3339),
		Alg:       alg,
		Signature: signatureB64,
	}
}

func (c Cert) ValidateBasic() error {
	if c.Issuer == "" {
		return errors.New("missing issuer")
	}
	if c.KeyID == "" {
		return errors.New("missing key_id")
	}
	if c.Alg == "" {
		return errors.New("missing alg")
	}
	if c.IssuedAt == "" || c.ExpiresAt == "" {
		return errors.New("missing issued_at/expires_at")
	}
	if c.Signature == "" {
		return errors.New("missing signature")
	}
	if _, err := base64.StdEncoding.DecodeString(c.Signature); err != nil {
		return fmt.Errorf("invalid signature encoding: %w", err)
	}
	return nil
}

