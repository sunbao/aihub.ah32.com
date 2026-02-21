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
			if err := ensurePublisherAgentsOffered(ctx, pool); err != nil {
				log.Printf("ensure publisher offers: %v", err)
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

func ensurePublisherAgentsOffered(ctx context.Context, pool *pgxpool.Pool) error {
	// MVP: make sure a publisher's own enabled agents can always see their run's work items,
	// even if the initial matching did not include them.
	//
	// Only touches unclaimed work items (status='offered'). Runs with required tags still
	// require the agent to have ALL required tags.
	_, err := pool.Exec(ctx, `
		with req as (
			select run_id, count(*)::int as req_count
			from run_required_tags
			group by run_id
		),
		target_work_items as (
			select
				wi.id as work_item_id,
				wi.run_id as run_id,
				r.publisher_user_id as owner_id,
				coalesce(req.req_count, 0) as req_count
			from work_items wi
			join runs r on r.id = wi.run_id
			left join req on req.run_id = r.id
			where wi.status = 'offered'
			  and wi.kind <> 'review'
			  and not exists (
				select 1
				from work_item_offers o
				join agents a0 on a0.id = o.agent_id
				where o.work_item_id = wi.id and a0.owner_id = r.publisher_user_id
			  )
		),
		eligible_owner_agents as (
			select
				tw.work_item_id as work_item_id,
				a.id as agent_id,
				tw.req_count as req_count
			from target_work_items tw
			join agents a on a.owner_id = tw.owner_id and a.status = 'enabled'
			left join run_required_tags rt on rt.run_id = tw.run_id
			left join agent_tags at on at.agent_id = a.id and at.tag = rt.tag
			group by tw.work_item_id, a.id, tw.req_count
			having tw.req_count = 0 or count(distinct at.tag) = tw.req_count
		)
		insert into work_item_offers (work_item_id, agent_id)
		select work_item_id, agent_id
		from eligible_owner_agents
		on conflict do nothing
	`)
	return err
}
