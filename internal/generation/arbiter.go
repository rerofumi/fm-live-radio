package generation

import (
	"context"
	"errors"
	"sync"
)

// Kind identifies the two local generation classes. Music has precedence over
// Talk when both classes are waiting; requests within a class remain FIFO.
type Kind uint8

const (
	KindTalk Kind = iota
	KindMusic
	// Talk and Music are concise aliases for callers that do not need the
	// Kind-prefixed names.
	Talk  = KindTalk
	Music = KindMusic
)

var ErrReservationConsumed = errors.New("generation reservation already consumed")

type request struct {
	kind    Kind
	granted chan struct{}
	wake    chan struct{}
	queued  bool
	started bool
}

// Arbiter serializes the process-local model runtime interval. A reservation
// enters the queue immediately, while the lease is held from just before
// runtime loading through runtime Close completion.
type Arbiter struct {
	mu     sync.Mutex
	active *request
	queue  []*request
}

func NewArbiter() *Arbiter { return &Arbiter{} }

var sharedArbiter = NewArbiter()

// SharedArbiter is the product-wide process-shared arbiter.
func SharedArbiter() *Arbiter { return sharedArbiter }

// Reserve registers a request on the product-wide arbiter. Coordinators may
// use this together with WithReservation before starting a service worker.
func Reserve(ctx context.Context, kind Kind) (*Reservation, error) {
	return sharedArbiter.Reserve(ctx, kind)
}

// Acquire is the convenience form for callers that do not need to pass a
// reservation through a coordinating context.
func (a *Arbiter) Acquire(ctx context.Context, kind Kind) (*Lease, error) {
	reservation, err := a.Reserve(ctx, kind)
	if err != nil {
		return nil, err
	}
	lease, err := reservation.Acquire(ctx)
	if err != nil {
		reservation.Release()
		return nil, err
	}
	return lease, nil
}

// Reservation is a single-use queued request. Release is safe before or after
// an attempted acquire and is intentionally idempotent for deferred cleanup.
type Reservation struct {
	arbiter  *Arbiter
	req      *request
	once     sync.Once
	stateMu  sync.Mutex
	acquired bool
}

// Arbiter reports the scheduler that owns this reservation, allowing services
// with an isolated test scheduler to reject a foreign context reservation.
func (r *Reservation) Arbiter() *Arbiter {
	if r == nil {
		return nil
	}
	return r.arbiter
}

// Reserve registers a request before potentially expensive service preflight.
func (a *Arbiter) Reserve(ctx context.Context, kind Kind) (*Reservation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r := &request{kind: kind, granted: make(chan struct{}), wake: make(chan struct{}), queued: true}
	a.mu.Lock()
	if err := ctx.Err(); err != nil {
		a.mu.Unlock()
		return nil, err
	}
	a.queue = append(a.queue, r)
	a.wakeLocked()
	a.mu.Unlock()
	return &Reservation{arbiter: a, req: r}, nil
}

// WithReservation passes a reservation from a coordinating caller to a
// service without changing the public service method signatures.
func WithReservation(ctx context.Context, reservation *Reservation) context.Context {
	if reservation == nil {
		return ctx
	}
	return context.WithValue(ctx, reservationContextKey{}, reservation)
}

// ReservationFromContext returns a matching reservation, if one was passed.
func ReservationFromContext(ctx context.Context, kind Kind) *Reservation {
	reservation, _ := ctx.Value(reservationContextKey{}).(*Reservation)
	if reservation == nil || reservation.req == nil || reservation.req.kind != kind {
		return nil
	}
	return reservation
}

type reservationContextKey struct{}

// Acquire waits until this request is selected. Cancellation removes a still
// waiting request. Once selected, cancellation never abandons the lease; the
// service must join its worker and close the runtime before Release.
func (r *Reservation) Acquire(ctx context.Context) (*Lease, error) {
	if r == nil || r.arbiter == nil || r.req == nil {
		return nil, errors.New("nil generation reservation")
	}
	a := r.arbiter
	for {
		a.mu.Lock()
		r.stateMu.Lock()
		if r.acquired {
			r.stateMu.Unlock()
			a.mu.Unlock()
			return nil, ErrReservationConsumed
		}
		if r.req.started {
			r.acquired = true
			r.stateMu.Unlock()
			a.mu.Unlock()
			return &Lease{arbiter: a, req: r.req}, nil
		}
		if !r.req.queued {
			r.stateMu.Unlock()
			a.mu.Unlock()
			return nil, ErrReservationConsumed
		}
		if err := ctx.Err(); err != nil {
			r.stateMu.Unlock()
			a.removeLocked(r.req)
			a.mu.Unlock()
			return nil, err
		}
		if a.active == nil && a.nextLocked() == r.req {
			a.removeLocked(r.req)
			r.req.started = true
			a.active = r.req
			r.acquired = true
			r.stateMu.Unlock()
			close(r.req.granted)
			a.wakeLocked()
			a.mu.Unlock()
			return &Lease{arbiter: a, req: r.req}, nil
		}
		r.stateMu.Unlock()
		wake := r.req.wake
		a.mu.Unlock()
		select {
		case <-wake:
		case <-ctx.Done():
		}
	}
}

// Release removes an unconsumed reservation. Once Acquire has handed out a
// lease, only that Lease may release the active slot.
func (r *Reservation) Release() {
	if r == nil || r.arbiter == nil || r.req == nil {
		return
	}
	r.once.Do(func() {
		r.arbiter.mu.Lock()
		if r.req.queued {
			r.arbiter.removeLocked(r.req)
		}
		r.arbiter.mu.Unlock()
	})
}

type Lease struct {
	arbiter *Arbiter
	req     *request
	once    sync.Once
}

func (l *Lease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.arbiter == nil || l.req == nil {
			return
		}
		l.arbiter.mu.Lock()
		if l.arbiter.active == l.req {
			l.arbiter.active = nil
			l.arbiter.wakeLocked()
		}
		l.arbiter.mu.Unlock()
	})
}

func (a *Arbiter) nextLocked() *request {
	for _, preferred := range []Kind{KindMusic, KindTalk} {
		for _, r := range a.queue {
			if r.queued && r.kind == preferred {
				return r
			}
		}
	}
	return nil
}

func (a *Arbiter) removeLocked(target *request) {
	if !target.queued {
		return
	}
	target.queued = false
	for i, r := range a.queue {
		if r == target {
			a.queue = append(a.queue[:i], a.queue[i+1:]...)
			break
		}
	}
	a.wakeLocked()
}

func (a *Arbiter) wakeLocked() {
	for _, r := range a.queue {
		close(r.wake)
		r.wake = make(chan struct{})
	}
}
