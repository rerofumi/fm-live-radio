package main

import (
	"strings"
	"testing"
	"time"
)

func TestDrainLifecycleEventsWaitsForCloseEndBarrier(t *testing.T) {
	events := make(chan string, 4)
	want := map[string]bool{"talk": true}
	lifecycle := []string{"talk:start inference", "talk:shutdown accepted"}
	go func() {
		events <- "talk:end inference"
		events <- "talk:service join"
		events <- "talk:start close"
		events <- "talk:end close"
	}()

	got, err := drainLifecycleEvents(events, lifecycle, want, time.Second)
	if err != nil {
		t.Fatalf("close-end barrier failed: %v", err)
	}
	if len(got) != 6 || got[len(got)-1] != "talk:end close" {
		t.Fatalf("lifecycle=%v, want complete ordered trace", got)
	}
}

func TestDrainLifecycleEventsFailsWithoutCloseEnd(t *testing.T) {
	events := make(chan string, 1)
	events <- "talk:service join"
	_, err := drainLifecycleEvents(events, []string{"talk:start inference"}, map[string]bool{"talk": true}, 5*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "talk:end close") {
		t.Fatalf("missing close end should fail with barrier reason, got %v", err)
	}
}
