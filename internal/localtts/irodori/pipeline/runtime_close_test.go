package pipeline

import (
	"sync"
	"testing"
)

func TestRuntimeCloseConcurrentCallersJoinSameCompletion(t *testing.T) {
	var mu sync.Mutex
	var events []RuntimeEvent
	closeWait := make(chan struct{})
	r := &Runtime{
		closeDone:     make(chan struct{}),
		closeWaitHook: func() { close(closeWait) },
		observer: func(event RuntimeEvent) {
			mu.Lock()
			events = append(events, event)
			mu.Unlock()
		},
	}
	r.inflight.Add(1)

	firstDone := make(chan struct{})
	secondDone := make(chan struct{})
	go func() { r.Close(); close(firstDone) }()
	<-closeWait
	// The first Close has emitted close-start and is blocked in inflight.Wait.
	go func() { r.Close(); close(secondDone) }()

	select {
	case <-firstDone:
		t.Fatal("first Close returned before in-flight work joined")
	case <-secondDone:
		t.Fatal("second Close returned before first Close completed")
	default:
	}
	r.inflight.Done()
	<-firstDone
	<-secondDone

	mu.Lock()
	defer mu.Unlock()
	if countEvent(events, EventCloseStart) != 1 || countEvent(events, EventCloseEnd) != 1 {
		t.Fatalf("close events = %v, want one start/end pair", events)
	}
}

func countEvent(events []RuntimeEvent, want RuntimeEvent) int {
	n := 0
	for _, event := range events {
		if event == want {
			n++
		}
	}
	return n
}
