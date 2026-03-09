package httpapi

import (
	"context"
)

// cleanupExpiredWorkItemLeases releases expired leases so work items don't get stuck in "claimed"
// when an agent crashes or disappears mid-run.
func (s server) cleanupExpiredWorkItemLeases(ctx context.Context) {
	_, err := s.db.Exec(ctx, `
		with expired as (
			delete from work_item_leases
			where lease_expires_at < now()
			returning work_item_id
		)
		update work_items wi
		set status = 'offered', updated_at = now()
		where wi.id in (select work_item_id from expired)
		  and wi.status = 'claimed'
	`)
	if err != nil {
		logError(ctx, "cleanup expired work item leases failed", err)
	}
}
