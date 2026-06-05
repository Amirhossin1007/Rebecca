package masterapi

import (
	"context"
	"log"
	"strings"
	"time"

	userapp "github.com/rebeccapanel/rebecca/go/internal/app/user"
)

const (
	defaultUserLifecycleInterval  = 30 * time.Second
	defaultUserUsageResetInterval = time.Hour
)

func (s *Server) runUserLifecycleWorkers(ctx context.Context) {
	go s.runUserLifecycleWorker(ctx)
	go s.runUserUsageResetWorker(ctx)
}

func (s *Server) runUserLifecycleWorker(ctx context.Context) {
	interval := parseWorkerInterval(s.cfg.UserLifecycleInterval, defaultUserLifecycleInterval)
	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.reviewUserLifecycle(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reviewUserLifecycle(ctx)
		}
	}
}

func (s *Server) reviewUserLifecycle(ctx context.Context) {
	workerCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	result, err := s.userService.ReviewLifecycle(workerCtx, userapp.LifecycleOptions{
		BatchSize: s.cfg.UserLifecycleBatchSize,
	})
	if err != nil {
		log.Printf("Go user lifecycle review failed: %v", err)
		return
	}
	if result.Limited > 0 || result.Expired > 0 || result.AppliedNextPlan > 0 || result.ActivatedOnHold > 0 {
		log.Printf(
			"Go user lifecycle checked_active=%d checked_on_hold=%d limited=%d expired=%d next_plan=%d activated_on_hold=%d",
			result.CheckedActive,
			result.CheckedOnHold,
			result.Limited,
			result.Expired,
			result.AppliedNextPlan,
			result.ActivatedOnHold,
		)
	}
}

func (s *Server) runUserUsageResetWorker(ctx context.Context) {
	interval := parseWorkerInterval(s.cfg.UserUsageResetInterval, defaultUserUsageResetInterval)
	if interval <= 0 {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	s.resetPeriodicUserUsage(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.resetPeriodicUserUsage(ctx)
		}
	}
}

func (s *Server) resetPeriodicUserUsage(ctx context.Context) {
	workerCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	result, err := s.userService.ResetPeriodicUsage(workerCtx, userapp.UsageResetOptions{
		BatchSize: s.cfg.UserUsageResetBatchSize,
	})
	if err != nil {
		log.Printf("Go periodic user usage reset failed: %v", err)
		return
	}
	if result.Reset > 0 {
		log.Printf(
			"Go periodic user usage reset checked=%d reset=%d reactivated=%d",
			result.Checked,
			result.Reset,
			result.Reactivated,
		)
	}
}

func parseWorkerInterval(value string, fallback time.Duration) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	if value == "0" || strings.EqualFold(value, "off") || strings.EqualFold(value, "false") {
		return 0
	}
	if duration, err := time.ParseDuration(value); err == nil {
		return duration
	}
	return fallback
}
