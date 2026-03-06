package httpapi

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s server) createRunInTx(ctx context.Context, tx pgx.Tx, publisherUserID uuid.UUID, goal, constraints string, requiredTags []string, scheduledAt *time.Time, isPublic bool) (runID uuid.UUID, runRef string, initialWorkItemID uuid.UUID, err error) {
	runID = uuid.Nil
	runRef = ""
	initialWorkItemID = uuid.Nil

	for attempt := 0; attempt < 5; attempt++ {
		ref, refErr := randomPublicRef(runRefPrefix)
		if refErr != nil {
			return uuid.Nil, "", uuid.Nil, refErr
		}
		runRef = ref
		insErr := tx.QueryRow(ctx, `
			insert into runs (public_ref, publisher_user_id, goal, constraints, status, review_status, is_public)
			values ($1, $2, $3, $4, 'created', 'approved', $5)
			returning id
		`, runRef, publisherUserID, goal, constraints, isPublic).Scan(&runID)
		if insErr == nil {
			break
		}
		if isUniqueViolation(insErr) {
			logError(ctx, "create run: run_ref collision on insert", insErr)
			continue
		}
		return uuid.Nil, "", uuid.Nil, insErr
	}
	if runID == uuid.Nil {
		return uuid.Nil, "", uuid.Nil, errors.New("create run failed (run_ref collision)")
	}

	for _, t := range requiredTags {
		if _, err := tx.Exec(ctx, `
			insert into run_required_tags (run_id, tag) values ($1, $2)
			on conflict do nothing
		`, runID, t); err != nil {
			return uuid.Nil, "", uuid.Nil, err
		}
	}

	workItemID, err := s.createInitialWorkItemAndOffers(ctx, tx, runID, publisherUserID, requiredTags, scheduledAt)
	if err != nil {
		return uuid.Nil, "", uuid.Nil, err
	}
	return runID, runRef, workItemID, nil
}
