package httpapi

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

func (s server) listOwnerAgentIDs(ctx context.Context, ownerID uuid.UUID, limit int) ([]uuid.UUID, error) {
	limit = clampInt(limit, 1, 200)
	rows, err := s.db.Query(ctx, `
		select id
		from agents
		where owner_id = $1
		order by updated_at desc
		limit $2
	`, ownerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]uuid.UUID, 0, limit)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s server) listOwnerAgentRefs(ctx context.Context, ownerID uuid.UUID, limit int) ([]string, error) {
	limit = clampInt(limit, 1, 200)
	rows, err := s.db.Query(ctx, `
		select public_ref
		from agents
		where owner_id = $1
		order by updated_at desc
		limit $2
	`, ownerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0, limit)
	for rows.Next() {
		var ref string
		if err := rows.Scan(&ref); err != nil {
			return nil, err
		}
		ref = strings.ToLower(strings.TrimSpace(ref))
		if ref == "" {
			continue
		}
		if _, err := parseAgentRef(ref); err != nil {
			logError(ctx, "list owner agent refs: invalid agent public_ref in db", err)
			continue
		}
		out = append(out, ref)
	}
	return out, rows.Err()
}
