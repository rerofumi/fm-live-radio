package generation

import (
	"context"
	"sync"
)

type Queue struct {
	sem chan struct{}
	wg  sync.WaitGroup
}

func NewQueue(workers int) *Queue {
	if workers <= 0 {
		workers = 1
	}
	return &Queue{sem: make(chan struct{}, workers)}
}

func (q *Queue) Go(ctx context.Context, fn func(context.Context)) {
	q.wg.Add(1)
	go func() {
		defer q.wg.Done()
		select {
		case q.sem <- struct{}{}:
		case <-ctx.Done():
			return
		}
		defer func() { <-q.sem }()
		fn(ctx)
	}()
}

func (q *Queue) Wait() {
	q.wg.Wait()
}
