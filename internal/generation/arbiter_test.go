package generation

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestArbiterMusicPriorityAndClassFIFO(t *testing.T) {
	a := NewArbiter()
	first, err := a.Acquire(context.Background(), KindTalk)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Release()

	var mu sync.Mutex
	var order []string
	started := make(chan struct{}, 3)
	reserve := func(kind Kind, name string) {
		r, err := a.Reserve(context.Background(), kind)
		if err != nil {
			t.Errorf("reserve %s: %v", name, err)
			return
		}
		go func() {
			lease, err := r.Acquire(context.Background())
			if err != nil {
				t.Errorf("acquire %s: %v", name, err)
				return
			}
			mu.Lock()
			order = append(order, name)
			mu.Unlock()
			started <- struct{}{}
			lease.Release()
		}()
	}
	reserve(KindTalk, "talk-1")
	reserve(KindTalk, "talk-2")
	reserve(KindMusic, "music-1")
	reserve(KindMusic, "music-2")

	first.Release()
	for i := 0; i < 4; i++ {
		<-started
	}
	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	want := []string{"music-1", "music-2", "talk-1", "talk-2"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order=%v, want %v", got, want)
		}
	}
}

func TestArbiterCancellationAndLeaseRelease(t *testing.T) {
	a := NewArbiter()
	lease, err := a.Acquire(context.Background(), KindTalk)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	r, err := a.Reserve(ctx, KindMusic)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := r.Acquire(ctx)
		done <- err
	}()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled acquire=%v, want context.Canceled", err)
	}
	r.Release()
	lease.Release()

	// A cancelled waiter must not block the next request after the active
	// lease is released.
	next, err := a.Acquire(context.Background(), KindTalk)
	if err != nil {
		t.Fatalf("next acquire after cancellation: %v", err)
	}
	next.Release()
}

func TestReservationIsSingleUseAndContextTransfer(t *testing.T) {
	a := NewArbiter()
	r, err := a.Reserve(context.Background(), KindMusic)
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithReservation(context.Background(), r)
	if got := ReservationFromContext(ctx, KindMusic); got != r {
		t.Fatal("context did not retain matching reservation")
	}
	lease, err := r.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Release()
	if _, err := r.Acquire(context.Background()); !errors.Is(err, ErrReservationConsumed) {
		t.Fatalf("second acquire=%v, want ErrReservationConsumed", err)
	}
	// Release is deliberately harmless after the lease has been handed out.
	r.Release()
}
