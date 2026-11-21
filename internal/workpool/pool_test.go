package workpool

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestBoundedFetchAndSerialPersistence(t *testing.T) {
	var active, peak atomic.Int32
	gate := make(chan struct{})
	items := make([]int, 20)
	for i := range items {
		items[i] = i
	}
	var written []int // deliberately not synchronized: apply must be serial
	done := make(chan error, 1)
	go func() {
		done <- Run(t.Context(), items, 3, func(ctx context.Context, i int) (int, error) {
			n := active.Add(1)
			defer active.Add(-1)
			for old := peak.Load(); n > old; old = peak.Load() {
				if peak.CompareAndSwap(old, n) {
					break
				}
			}
			select {
			case <-gate:
			case <-ctx.Done():
				return 0, ctx.Err()
			}
			return i, nil
		}, func(_ int, i int) error { written = append(written, i); return nil })
	}()
	deadline := time.After(time.Second)
	for peak.Load() < 3 {
		select {
		case <-deadline:
			t.Fatal("workers did not start")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if peak.Load() != 3 || len(written) != len(items) {
		t.Fatalf("peak=%d writes=%d", peak.Load(), len(written))
	}
}

func TestCancellationAndErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := Run(ctx, []int{1, 2, 3}, 2, func(ctx context.Context, _ int) (int, error) { return 0, ctx.Err() }, func(int, int) error { t.Error("wrote after cancellation"); return nil })
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	written := 0
	want := errors.New("fetch failed")
	err = Run(t.Context(), []int{1, 2}, 2, func(_ context.Context, i int) (int, error) {
		if i == 1 {
			return 0, want
		}
		return i, nil
	}, func(int, int) error { written++; return nil })
	if !errors.Is(err, want) || written != 1 {
		t.Fatalf("error=%v writes=%d", err, written)
	}
}

func TestPacerSpacesWaiters(t *testing.T) {
	p := &Pacer{}
	gap := 30 * time.Millisecond
	if err := p.Wait(t.Context(), gap); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := p.Wait(t.Context(), gap); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < gap-10*time.Millisecond {
		t.Fatalf("second waiter started after %v, want ~%v", elapsed, gap)
	}
}

func TestPacerGivesBackCancelledSlot(t *testing.T) {
	p := &Pacer{}
	gap := 50 * time.Millisecond
	if err := p.Wait(t.Context(), gap); err != nil {
		t.Fatal(err)
	}
	cancelCtx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- p.Wait(cancelCtx, gap) }()
	time.Sleep(5 * time.Millisecond) // let the waiter take its slot
	cancel()
	if err := <-done; err == nil {
		t.Fatal("cancelled wait succeeded")
	}
	// The cancelled slot was released: the next waiter starts within one
	// gap of the first, not two.
	start := time.Now()
	if err := p.Wait(t.Context(), gap); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > gap {
		t.Fatalf("next waiter waited %v; the cancelled slot was not released", elapsed)
	}
}
