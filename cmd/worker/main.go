package main

import (
	"context"
	"log"
	"os/signal"
	"syscall"
	"time"

	"aihub/internal/config"
	"aihub/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	pool, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pool.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ticker := time.NewTicker(time.Duration(cfg.WorkerTickSeconds) * time.Second)
	defer ticker.Stop()

	log.Printf("worker started (tick=%ds)", cfg.WorkerTickSeconds)

	for {
		select {
		case <-ctx.Done():
			log.Printf("worker stopping")
			return
		case <-ticker.C:
			if err := reclaimExpiredLeases(ctx, pool); err != nil {
				log.Printf("reclaim expired leases: %v", err)
			}
		}
	}
}

func reclaimExpiredLeases(ctx context.Context, pool *pgxpool.Pool) error {
	// Best-effort MVP: make expired claimed work items available again.
	_, err := pool.Exec(ctx, `
		with expired as (
			select work_item_id
			from work_item_leases
			where lease_expires_at < now()
		),
		del as (
			delete from work_item_leases l
			using expired e
			where l.work_item_id = e.work_item_id
			returning l.work_item_id
		)
		update work_items wi
		set status = 'offered', updated_at = now()
		where wi.id in (select work_item_id from del) and wi.status = 'claimed'
	`)
	if err != nil {
		return err
	}

	// Note: audit logging could be added later (task 10.2).
	return nil
}
