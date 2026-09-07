package ttseval

import "testing"

func TestFixedScriptsMeetBenchmarkContract(t *testing.T) {
	if err := ValidateAll(); err != nil {
		t.Fatal(err)
	}
	if got := len(Scripts()); got != 10 {
		t.Fatalf("scripts=%d, want 10", got)
	}
	for _, script := range Scripts() {
		if err := Validate(script); err != nil {
			t.Fatal(err)
		}
	}
}

func TestExpectedReadingsCoverAllSourceScripts(t *testing.T) {
	for _, s := range Scripts() {
		if s.ExpectedReading == "" {
			t.Fatalf("script %s has no expected reading", s.ID)
		}
	}
	for _, s := range Scripts() {
		if s.ID == "04" && s.ExpectedReading != "しゅうまつのしんかんせん、しんおおさか、みっかまえ" {
			t.Fatalf("script 04 expected reading=%q", s.ExpectedReading)
		}
	}
}
