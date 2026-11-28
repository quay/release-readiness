package artbuild

import (
	"context"
	"errors"
	"fmt"
	"github.com/quay/release-readiness/internal/syncstatus"
	"slices"
	"strings"
	"time"
)

// syncPrioritized reports current-release freshness before best-effort history.
// Both phases are bounded; partial days are never marked complete.
func (h *HistorySync) syncPrioritized(ctx context.Context) {
	started := time.Now()
	activity, err := h.Activity(ctx)
	if err != nil {
		h.logger.Warn("ART priority metadata unavailable; treating configured groups as current", "error", err)
		activity = nil
	}
	var active, historical []string
	var discovery syncstatus.Pass
	for _, group := range h.unionGroups(ctx, &discovery) {
		app := strings.ReplaceAll(group, ".", "-")
		if current, known := activity[app]; !known || current {
			active = append(active, group)
		} else {
			historical = append(historical, group)
		}
	}
	if len(active) == 0 && len(historical) == 0 {
		discovery.Add(errWaitingForStreams)
		h.Status.Report(discovery.Err())
		h.logger.Info("ART waiting for stream discovery")
		return
	}
	slices.Sort(active)
	slices.Sort(historical)
	if len(active) > 0 {
		offset := h.foregroundCursor % len(active)
		h.foregroundCursor++
		active = append(slices.Clone(active[offset:]), active[:offset]...)
	}
	h.logger.Info("ART scheduling", "foreground", active, "historical", historical)
	budget := h.ForegroundBudget
	if budget <= 0 {
		budget = 2 * time.Minute
	}
	current, cancel := context.WithTimeout(ctx, budget)
	var pass syncstatus.Pass
	scanned := 0
	for day := range refreshDays {
		for _, group := range active {
			if current.Err() != nil {
				break
			}
			err := h.scanAndMark(current, group, h.now().UTC().AddDate(0, 0, -day).Format(time.DateOnly))
			pass.Add(err)
			if err == nil {
				scanned++
			}
		}
	}
	pass.Add(current.Err())
	cancel()
	if len(active) > 0 {
		h.Status.Report(pass.Err())
		h.logger.Info("ART foreground synced", "duration_ms", time.Since(started).Milliseconds(), "days_scanned", scanned, "failed", pass.Err() != nil)
	} else {
		h.Status.Idle("Only historical ART groups are configured")
		h.logger.Info("ART foreground idle", "reason", "only historical streams discovered")
	}
	if pass.Err() != nil || ctx.Err() != nil {
		return
	}
	if h.BackfillInterval <= 0 || (!h.lastBackfill.IsZero() && h.now().Sub(h.lastBackfill) < h.BackfillInterval) {
		return
	}
	// Active groups backfill on every visit, so a candidate FBC older than
	// the refresh window is stored without waiting for the rotation;
	// historical-only groups rotate one per visit.
	targets := slices.Clone(active)
	if len(historical) > 0 {
		targets = append(targets, historical[h.backfillCursor%len(historical)])
		h.backfillCursor++
	}
	if len(targets) == 0 {
		return
	}
	budget = h.BackfillBudget
	if budget <= 0 {
		budget = 30 * time.Second
	}
	backfill, stop := context.WithTimeout(ctx, budget)
	started = time.Now()
	var backfillPass syncstatus.Pass
	for _, group := range targets {
		backfillPass.Add(h.backfillGroup(backfill, group, slices.Contains(active, group)))
		if errors.Is(backfill.Err(), context.DeadlineExceeded) && ctx.Err() == nil {
			break
		}
	}
	// Source metadata is best effort too; it cannot extend the history budget.
	if backfillPass.Err() == nil {
		var sources syncstatus.Pass
		h.backfillSources(backfill, &sources)
		backfillPass.Add(errors.Join(sources.Err(), backfill.Err()))
	}
	budgetExhausted := errors.Is(backfill.Err(), context.DeadlineExceeded) && ctx.Err() == nil
	stop()
	h.lastBackfill = h.now()
	if budgetExhausted {
		h.logger.Info("ART backfill paused", "groups", targets, "duration_ms", time.Since(started).Milliseconds(), "reason", "time budget exhausted")
		return
	}
	h.BackfillStatus.Report(backfillPass.Err())
	h.logger.Info("ART backfill synced", "groups", targets, "duration_ms", time.Since(started).Milliseconds(), "failed", backfillPass.Err() != nil)
	if backfillPass.Err() != nil {
		h.logger.Warn("ART backfill incomplete", "error", backfillPass.Err())
	}
}

func (h *HistorySync) scanAndMark(ctx context.Context, group, day string) error {
	if err := h.scanDay(ctx, group, day); err != nil {
		return fmt.Errorf("scan %s %s: %w", group, day, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return h.store.MarkArtHistoryDayScanned(ctx, group, TypeFBC, day, h.now())
}

func (h *HistorySync) backfillGroup(ctx context.Context, group string, active bool) error {
	scanned, err := h.store.ScannedArtHistoryDays(ctx, group, TypeFBC)
	if err != nil {
		return err
	}
	// Current streams already scanned the newest days in the foreground phase.
	first := 0
	if active {
		first = refreshDays
	}
	count := 0
	var failed error
	for day := first; day < historyDays && count < backfillPerPass; day++ {
		date := h.now().UTC().AddDate(0, 0, -day).Format(time.DateOnly)
		if day >= refreshDays && slices.Contains(scanned, date) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return errors.Join(failed, err)
		}
		if err := h.scanAndMark(ctx, group, date); err != nil {
			if ctx.Err() != nil {
				return errors.Join(failed, err)
			}
			// A failed day is not marked scanned, so a later visit retries
			// it, but this visit reads on: the day still counts toward the
			// per-pass budget.
			failed = errors.Join(failed, err)
		}
		count++
	}
	return errors.Join(failed, ctx.Err())
}
