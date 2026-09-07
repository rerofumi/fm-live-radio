package sampler

import "testing"

func TestV4CFGWindowAndIndependentBundles(t *testing.T) {
	c := Defaults()
	if !c.CfgActive(0.5) || !c.CfgActive(1.0) || c.CfgActive(0.49) || c.CfgActive(1.01) {
		t.Fatal("CFG window must be inclusive 0.5..1.0")
	}
	c.UseSpeakerCfg = true
	b := c.ActiveBundles()
	if len(b) != 4 || b[0] != BundleCond || b[1] != BundleNoText || b[2] != BundleNoSpeaker || b[3] != BundleNoCaption {
		t.Fatalf("unexpected bundles: %v", b)
	}
}

func TestV4CFGApplyScaleOne(t *testing.T) {
	c := Defaults()
	c.ScaleText = 1
	c.UseCaptionCfg = false
	c.UseSpeakerCfg = false
	got := ApplyCFG(c, []float32{2}, []float32{1}, nil, nil)
	if len(got) != 1 || got[0] != 3 {
		t.Fatalf("got %v, want 3", got)
	}
}
