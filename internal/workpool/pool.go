// Package workpool bounds background fetches while serializing persistence.
package workpool

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Run fetches at most workers items concurrently. apply runs on the caller's
// goroutine, outside fetches, so stores need not support concurrent writes.
// The bounded channels provide backpressure; cancellation drains all workers.
func Run[T, R any](ctx context.Context, items []T, workers int, fetch func(context.Context, T) (R, error), apply func(T, R) error) error {
	workers = min(max(workers, 1), len(items))
	if workers == 0 {
		return ctx.Err()
	}
	type result struct {
		item  T
		value R
		err   error
	}
	jobs := make(chan T)
	results := make(chan result, workers)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for item := range jobs {
				if ctx.Err() != nil {
					return
				}
				value, err := fetch(ctx, item)
				select {
				case results <- result{item, value, err}:
				case <-ctx.Done():
					return
				}
			}
		})
	}
	go func() {
		defer close(jobs)
		for _, item := range items {
			select {
			case jobs <- item:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() { wg.Wait(); close(results) }()
	var errs []error
	for r := range results {
		if r.err != nil {
			errs = append(errs, r.err)
			continue
		}
		if ctx.Err() == nil {
			if err := apply(r.item, r.value); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(append(errs, ctx.Err())...)
}

// Pacer shares a minimum start interval across all requests using a client.
// It is safe for concurrent fetch workers. No goroutine or ticker is retained.
type Pacer struct {
	mu   sync.Mutex
	next time.Time
}

func (p *Pacer) Wait(ctx context.Context, gap time.Duration) error {
	if gap <= 0 {
		return ctx.Err()
	}
	p.mu.Lock()
	at := maxTime(time.Now(), p.next)
	p.next = at.Add(gap)
	p.mu.Unlock()
	timer := time.NewTimer(time.Until(at))
	defer timer.Stop()
	select {
	case <-timer.C:
		return ctx.Err()
	case <-ctx.Done():
		p.release(at, gap)
		return ctx.Err()
	}
}

// release gives a cancelled waiter's slot back, but only while no later
// reservation exists: a slot nobody used must not delay the callers behind it.
func (p *Pacer) release(at time.Time, gap time.Duration) {
	p.mu.Lock()
	if p.next.Equal(at.Add(gap)) {
		p.next = at
	}
	p.mu.Unlock()
}

func maxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}
