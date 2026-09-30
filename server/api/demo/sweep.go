package demo

import (
	"context"
	"log"
	"time"

	"quartz/config"
	"quartz/internal/model"

	"github.com/bsm/redislock"
)

func (h *Handler) StartSweepScheduler(ctx context.Context) {
	if config.Cfg.Sweeper.Disabled {
		log.Println("Sweeper is disabled via config (demo sweeper)")
		return
	}

	log.Println("Starting Demo Sweep Scheduler (waiting for master lock)...")

	locker := redislock.New(h.redis)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			lock, err := locker.Obtain(ctx, "sweep:scheduler:demo:master", 15*time.Second, nil)
			if err == redislock.ErrNotObtained {
				time.Sleep(5 * time.Second)
				continue
			} else if err != nil {
				log.Printf("Error obtaining demo sweep lock: %v", err)
				time.Sleep(5 * time.Second)
				continue
			}

			log.Println("Acquired master demo sweep lock! Proceeding with scheduler...")
			h.runDemoSweepLoopWithHeartbeat(ctx, lock)
		}
	}()
}

func (h *Handler) runDemoSweepLoopWithHeartbeat(ctx context.Context, lock *redislock.Lock) {
	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-hbCtx.Done():
				return
			case <-ticker.C:
				if err := lock.Refresh(hbCtx, 15*time.Second, nil); err != nil {
					log.Printf("Failed to refresh demo sweep master lock: %v. Relinquishing master role.", err)
					hbCancel()
					return
				}
			}
		}
	}()

	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()

	// Run an initial sweep immediately on startup
	log.Println("Running initial demo sweep on startup...")
	h.SweepExpiredDemoUsers(hbCtx)

	for {
		select {
		case <-hbCtx.Done():
			log.Println("Lost master demo sweep lock or shutting down. Stopping demo sweeper.")
			return
		case <-ticker.C:
			log.Println("15-minute demo sweep triggered")
			h.SweepExpiredDemoUsers(hbCtx)
		}
	}
}

func (h *Handler) SweepExpiredDemoUsers(ctx context.Context) {
	log.Println("SweepExpiredDemoUsers started")
	var expiredUsers []model.User

	// Find demo users older than 1 hour
	if err := h.db.Where("is_demo = ? AND created_at <= ? AND id != ?", true, time.Now().Add(-1*time.Hour), "00000000-0000-0000-0000-000000000000").Find(&expiredUsers).Error; err != nil {
		log.Printf("SweepExpiredDemoUsers error: failed to query expired demo users: %v", err)
		return
	}

	if len(expiredUsers) == 0 {
		log.Println("SweepExpiredDemoUsers finished (0 users to clean up)")
		return
	}

	for _, u := range expiredUsers {
		log.Printf("Sweeping demo user: %s (created %s)", u.ID, u.CreatedAt)
		if err := h.cleanupDemoUser(ctx, u.ID); err != nil {
			log.Printf("Failed to cleanup demo user %s: %v", u.ID, err)
		} else {
			log.Printf("Successfully swept demo user %s", u.ID)
		}
	}

	log.Printf("SweepExpiredDemoUsers finished: processed %d users", len(expiredUsers))
}
