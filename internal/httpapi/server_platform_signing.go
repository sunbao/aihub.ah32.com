package httpapi

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"net/http"
	"sort"
	"strings"
	"time"

	"aihub/internal/agenthome"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type platformSigningKeyDTO struct {
	KeyID     string `json:"key_id"`
	Alg       string `json:"alg"`
	PublicKey string `json:"public_key"`
	CreatedAt string `json:"created_at,omitempty"`
	RevokedAt string `json:"revoked_at,omitempty"`
}

func (s server) handleListPlatformSigningKeys(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, `
		select key_id, alg, public_key
		from platform_signing_keys
		where revoked_at is null
		order by created_at desc
	`)
	if err != nil {
		logError(ctx, "query platform_signing_keys failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	var keys []platformSigningKeyDTO
	for rows.Next() {
		var k platformSigningKeyDTO
		if err := rows.Scan(&k.KeyID, &k.Alg, &k.PublicKey); err != nil {
			logError(ctx, "scan platform_signing_keys failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "iterate platform_signing_keys failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (s server) handleAdminListPlatformSigningKeys(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	rows, err := s.db.Query(ctx, `
		select key_id, alg, public_key, created_at, revoked_at
		from platform_signing_keys
		order by created_at desc
	`)
	if err != nil {
		logError(ctx, "query platform_signing_keys failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "query failed"})
		return
	}
	defer rows.Close()

	var keys []platformSigningKeyDTO
	for rows.Next() {
		var (
			k         platformSigningKeyDTO
			createdAt time.Time
			revokedAt *time.Time
		)
		if err := rows.Scan(&k.KeyID, &k.Alg, &k.PublicKey, &createdAt, &revokedAt); err != nil {
			logError(ctx, "scan platform_signing_keys failed", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "scan failed"})
			return
		}
		k.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		if revokedAt != nil {
			k.RevokedAt = revokedAt.UTC().Format(time.RFC3339)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		logError(ctx, "iterate platform_signing_keys failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "iterate failed"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (s server) handleAdminRotatePlatformSigningKey(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.platformKeysEncryptionKey) == "" {
		writeJSON(w, http.StatusPreconditionFailed, map[string]string{"error": "missing platform key encryption config"})
		return
	}

	publicKey, privateKey, err := agenthome.GenerateEd25519Keypair()
	if err != nil {
		logError(r.Context(), "generate ed25519 keypair failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "key generation failed"})
		return
	}

	keyID := "platform_ed25519_" + time.Now().UTC().Format("20060102_150405") + "_" + uuid.New().String()[:8]
	privateEnc, err := agenthome.EncryptForDB(s.platformKeysEncryptionKey, privateKey)
	if err != nil {
		logError(r.Context(), "encrypt private key failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "encrypt failed"})
		return
	}

	pubB64 := base64.StdEncoding.EncodeToString(publicKey)

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if _, err := s.db.Exec(ctx, `
		insert into platform_signing_keys (key_id, alg, public_key, private_key_enc)
		values ($1, 'Ed25519', $2, $3)
	`, keyID, pubB64, privateEnc); err != nil {
		logError(ctx, "insert platform_signing_keys failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "insert failed"})
		return
	}

	s.audit(ctx, "admin", uuid.Nil, "platform_signing_key_rotated", map[string]any{"key_id": keyID})
	writeJSON(w, http.StatusCreated, map[string]any{
		"key_id":     keyID,
		"alg":        "Ed25519",
		"public_key": pubB64,
	})
}

func (s server) handleAdminRevokePlatformSigningKey(w http.ResponseWriter, r *http.Request) {
	keyID := strings.TrimSpace(chi.URLParam(r, "keyID"))
	if keyID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid key id"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	ct, err := s.db.Exec(ctx, `update platform_signing_keys set revoked_at = now() where key_id = $1 and revoked_at is null`, keyID)
	if err != nil {
		logError(ctx, "revoke platform_signing_keys failed", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "update failed"})
		return
	}
	if ct.RowsAffected() == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	s.audit(ctx, "admin", uuid.Nil, "platform_signing_key_revoked", map[string]any{"key_id": keyID})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s server) getActivePlatformSigningKey(ctx context.Context) (keyID string, alg string, publicKey ed25519.PublicKey, privateKey ed25519.PrivateKey, err error) {
	if strings.TrimSpace(s.platformKeysEncryptionKey) == "" {
		return "", "", nil, nil, agenthome.ErrMissingEncryptionKey
	}

	var (
		pubB64    string
		privEnc   []byte
	)
	row := s.db.QueryRow(ctx, `
		select key_id, alg, public_key, private_key_enc
		from platform_signing_keys
		where revoked_at is null
		order by created_at desc
		limit 1
	`)
	if err := row.Scan(&keyID, &alg, &pubB64, &privEnc); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", nil, nil, errors.New("no active platform signing keys (rotate one in /v1/admin/platform/signing-keys/rotate)")
		}
		return "", "", nil, nil, err
	}

	pubRaw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(pubB64))
	if err != nil {
		return "", "", nil, nil, err
	}
	if len(pubRaw) != ed25519.PublicKeySize {
		return "", "", nil, nil, errors.New("invalid platform public key length")
	}
	publicKey = ed25519.PublicKey(pubRaw)

	privRaw, err := agenthome.DecryptFromDB(s.platformKeysEncryptionKey, privEnc)
	if err != nil {
		return "", "", nil, nil, err
	}
	if len(privRaw) != ed25519.PrivateKeySize {
		return "", "", nil, nil, errors.New("invalid platform private key length")
	}
	privateKey = ed25519.PrivateKey(privRaw)
	return keyID, alg, publicKey, privateKey, nil
}

func (s server) signObject(ctx context.Context, obj map[string]any) (agenthome.Cert, error) {
	keyID, alg, _, priv, err := s.getActivePlatformSigningKey(ctx)
	if err != nil {
		return agenthome.Cert{}, err
	}

	signable := make(map[string]any, len(obj))
	for k, v := range obj {
		if k == "cert" {
			continue
		}
		signable[k] = v
	}

	canonical, err := agenthome.CanonicalJSON(signable)
	if err != nil {
		return agenthome.Cert{}, err
	}
	sig, err := agenthome.SignEd25519Base64(priv, canonical)
	if err != nil {
		return agenthome.Cert{}, err
	}
	issuedAt := time.Now().UTC()
	expiresAt := issuedAt.Add(time.Duration(s.platformCertTTLSeconds) * time.Second)
	return agenthome.NewCert(s.platformCertIssuer, keyID, alg, issuedAt, expiresAt, sig), nil
}

func sortStringsUnique(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
