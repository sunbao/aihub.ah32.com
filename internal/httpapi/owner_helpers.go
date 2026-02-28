package httpapi

import (
	"context"

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
