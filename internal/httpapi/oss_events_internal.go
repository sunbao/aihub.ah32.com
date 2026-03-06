package httpapi

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s server) insertOSSEvent(ctx context.Context, objectKey string, eventType string, occurredAt time.Time, payloadJSON []byte) error {
	_, err := s.db.Exec(ctx, `
		insert into oss_events (object_key, event_type, occurred_at, payload)
		values ($1, $2, $3, $4)
	`, objectKey, eventType, occurredAt.UTC(), payloadJSON)
	return err
}

func insertOSSEventInTx(ctx context.Context, tx pgx.Tx, objectKey string, eventType string, occurredAt time.Time, payloadJSON []byte) error {
	_, err := tx.Exec(ctx, `
		insert into oss_events (object_key, event_type, occurred_at, payload)
		values ($1, $2, $3, $4)
	`, objectKey, eventType, occurredAt.UTC(), payloadJSON)
	return err
}
