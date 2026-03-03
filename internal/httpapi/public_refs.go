package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	agentRefPrefix         = "a_"
	runRefPrefix           = "r_"
	publicRefTokenHexChars = 16
)

func stablePublicRefFromUUID(prefix string, id uuid.UUID) string {
	sum := sha256.Sum256([]byte(id.String()))
	return prefix + hex.EncodeToString(sum[:])[:publicRefTokenHexChars]
}

func randomPublicRef(prefix string) (string, error) {
	buf := make([]byte, publicRefTokenHexChars/2)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(buf), nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
