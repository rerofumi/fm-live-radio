package pipeline

import "testing"

func TestRuntimeCloseConcurrentCallersJoinSameCompletion(t *testing.T) {
	closeWait := make(chan struct{})
	r := &Runtime{closeDone: make(chan struct{}), closeWaitHook: func() { close(closeWait) }}
	r.active.Add(1)
	firstDone := make(chan struct{})
	secondDone := make(chan struct{})
	go func() { r.Close(); close(firstDone) }()
	<-closeWait
	// The first Close is now blocked in active.Wait; only then start the second.
	go func() { r.Close(); close(secondDone) }()
	select {
	case <-firstDone:
		t.Fatal("first Close returned before in-flight work joined")
	case <-secondDone:
		t.Fatal("second Close returned before first Close completed")
	default:
	}
	r.active.Done()
	<-firstDone
	<-secondDone
}
