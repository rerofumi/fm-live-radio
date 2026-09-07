// tts-e2e runs a network-independent RSS/LLM -> v4 ONNX -> WAV -> local
// audio-server flow.  The HTTP fixture is httptest on loopback; no external
// news or LLM endpoint is contacted.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"fm-live-radio/internal/audio"
	"fm-live-radio/internal/domain"
	"fm-live-radio/internal/generation"
	"fm-live-radio/internal/localtts"
	irodoriPipeline "fm-live-radio/internal/localtts/irodori/pipeline"
	"fm-live-radio/internal/musicgen"
	stablepipeline "fm-live-radio/internal/musicgen/stableaudio/pipeline"
	"fm-live-radio/internal/player"
	"fm-live-radio/internal/store"
	"fm-live-radio/internal/talk"
	"fm-live-radio/internal/ttseval"
)

type result struct {
	Version         string             `json:"version"`
	EP              string             `json:"execution_provider"`
	Model           string             `json:"model"`
	Cycles          int                `json:"cycles"`
	RSSLLMFixture   bool               `json:"rss_llm_fixture"`
	BGMProvider     string             `json:"bgm_provider"`
	CyclesOK        int                `json:"cycles_ok"`
	AudioURLs       []string           `json:"audio_urls"`
	LoudnessURLs    []string           `json:"loudness_urls"`
	Preflight       map[string]string  `json:"preflight_cases"`
	Config          map[string]any     `json:"isolated_config_restart"`
	Shutdown        map[string]any     `json:"shutdown"`
	Events          []string           `json:"events"`
	Actions         map[string]bool    `json:"actions"`
	Errors          []string           `json:"errors,omitempty"`
	Snapshot        string             `json:"snapshot"`
	Parent          string             `json:"parent"`
	Steps           int                `json:"steps"`
	Seconds         float64            `json:"seconds"`
	CFG             map[string]float64 `json:"cfg"`
	DurationScale   float64            `json:"duration_scale"`
	ReferenceSHA256 string             `json:"reference_sha256"`
	InputSHA256     string             `json:"input_sha256"`
	ModelAssets     map[string]string  `json:"model_assets"`
	ORTLibrary      string             `json:"ort_library"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	model := flag.String("model", "model/irodori-v4.1", "v4.1 model directory")
	ep := flag.String("ep", "cpu", "cpu, cuda, or auto; use another process per EP")
	ref := flag.String("ref", "narrator/narrator_01.wav", "reference WAV")
	steps := flag.Int("steps", 40, "Irodori denoising steps; use 2 only for diagnostic smoke")
	seconds := flag.Float64("seconds", -1, "Irodori output seconds; -1 uses duration predictor")
	bgmModel := flag.String("bgm-model", "model/sa3-sm-music", "Stable Audio 3 model directory")
	bgmSteps := flag.Int("bgm-steps", 1, "Stable Audio denoising steps for E2E smoke")
	bgmSeconds := flag.Float64("bgm-seconds", 1, "Stable Audio output seconds for E2E smoke")
	cycles := flag.Int("cycles", 3, "RSS/Talk/BGM cycles")
	out := flag.String("out", filepath.Join("evidence", "tts-e2e"), "report/output directory")
	configChild := flag.Bool("config-child", false, "internal isolated config restart child")
	configDir := flag.String("config-dir", "", "internal isolated config directory")
	flag.Parse()
	if *configChild {
		return runConfigChild(*configDir, *ep, *ref, *steps, *seconds)
	}
	if *cycles < 3 {
		return errors.New("REQ-10 requires at least 3 cycles")
	}
	if *ep != "cpu" && *ep != "cuda" && *ep != "auto" {
		return fmt.Errorf("unsupported EP %q", *ep)
	}
	if err := ttseval.ValidateAll(); err != nil {
		return err
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	lib := generation.ResolveORTLibraryPathForEP(*ep)
	if lib == "" {
		return fmt.Errorf("matching %s ORT DLL not found", *ep)
	}
	if err := generation.ConfigureExecutionProvider(*ep, 0); err != nil {
		return err
	}
	if err := generation.Init(lib); err != nil {
		return err
	}
	fixtureText := ttseval.Scripts()[0].Text
	fixture := newFixtureServer(fixtureText)
	defer fixture.Close()
	audioServer, err := audio.Start()
	if err != nil {
		return err
	}
	defer audioServer.Close(context.Background())
	tmp, err := os.MkdirTemp("", "fm-radio-e2e-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	cfg := domain.AppConfig{RSSUrls: []string{fixture.URL + "/feed.xml"}, Talk: domain.TalkConfig{Enabled: true, CycleBgmCount: 2}, LLM: domain.LLMConfig{Enabled: true, BaseURL: fixture.URL, Model: "fixture"}, Irodori: domain.IrodoriConfig{ModelDir: *model, NarratorDir: filepath.Dir(*ref), RefWAV: *ref, Seconds: *seconds, NumSteps: *steps, SeedMode: "fixed", FixedSeed: 0, CfgText: 3, CfgCaption: 3, CfgSpeaker: 5, DurationScale: 1}, StableAudio3: domain.StableAudio3Config{ModelDir: *bgmModel, OutputDir: filepath.Join(tmp, "stable-audio"), PromptBase: "instrumental background music for a radio show, no vocals", Genre: "chill lo-fi", Seconds: *bgmSeconds, Steps: *bgmSteps, SeedMode: "fixed", FixedSeed: 0, CacheLimit: 3}, LocalInference: domain.LocalInferenceConfig{ORTLibraryPath: lib, ExecutionProvider: *ep}}
	snap, parent := vcsIDs()
	r := result{Version: "working-tree", EP: *ep, Model: *model, Cycles: *cycles, RSSLLMFixture: true, BGMProvider: "stable_audio_3", AudioURLs: []string{}, LoudnessURLs: []string{}, Preflight: map[string]string{}, Events: []string{}, Actions: map[string]bool{}, Snapshot: snap, Parent: parent, Steps: *steps, Seconds: *seconds, CFG: map[string]float64{"text": 3, "caption": 3, "speaker": 5}, DurationScale: 1, ReferenceSHA256: hashFile(*ref), InputSHA256: hashScripts(ttseval.Scripts()), ModelAssets: hashModelAssets(*model), ORTLibrary: lib}
	// Save -> process restart equivalent -> identify v4 -> synthesize -> save
	// v3 path -> restart equivalent.  The temp store is never the user store.
	if err := isolatedConfigRoundTrip(tmp, cfg, *model, *ep, *steps, *seconds); err != nil {
		r.Errors = append(r.Errors, err.Error())
	} else {
		r.Config = map[string]any{"saved_v4": true, "reloaded_v4_model": *model, "restored_v3": true, "isolated_dir": tmp, "child_process_v4": true, "child_process_v3": true}
	}
	// Product Player flow. RSS/LLM are loopback fixtures, while BGM is generated
	// by the real Stable Audio service and Talk by the real localtts Service.
	talkSvc := talk.New(filepath.Join(tmp, "temp_audio"))
	musicSvc := musicgen.New()
	playerSvc := player.New(cfg)
	defer playerSvc.Shutdown()
	used := map[string]bool{}
	seenBGM, seenTalk := false, false
	talkComplete := false
	for attempts := 0; attempts < *cycles*20 && r.CyclesOK < *cycles; attempts++ {
		item, nextHist, _, e := playerSvc.NextItem(audioServer, talkSvc, musicSvc, domain.NextItemRequest{}, domain.History{UsedArticleUrls: mapKeys(used)})
		if e != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("player attempt %d: %v", attempts+1, e))
			break
		}
		used = mapFromHistory(nextHist)
		r.Events = append(r.Events, fmt.Sprintf("attempt=%d kind=%s", attempts+1, item.Kind))
		if item.Kind != domain.PlayableKindBGM && item.Kind != domain.PlayableKindTalk {
			continue
		}
		if e = checkHTTP200(item.URL); e != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("%s: %v", item.Kind, e))
			break
		}
		loud := audioServer.LoudnessURLForAudioURL(item.URL)
		if e = checkLoudness(loud); e != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("%s loudness: %v", item.Kind, e))
			break
		}
		r.AudioURLs = append(r.AudioURLs, item.URL)
		r.LoudnessURLs = append(r.LoudnessURLs, loud)
		if item.Kind == domain.PlayableKindTalk {
			seenTalk = true
			talkComplete = true
			r.Events = append(r.Events, fmt.Sprintf("talk_complete=%d", r.CyclesOK+1))
		} else {
			seenBGM = true
			if talkComplete {
				r.CyclesOK++
				talkComplete = false
				r.Events = append(r.Events, fmt.Sprintf("bgm_resume=%d", r.CyclesOK))
			}
		}
	}
	if !seenBGM || !seenTalk {
		r.Errors = append(r.Errors, "3-cycle run did not observe both Stable Audio BGM and Talk")
	}
	if item, _, _, e := playerSvc.Skip(audioServer, talkSvc, musicSvc, domain.SkipRequest{CurrentKind: domain.PlayableKindTalk}, domain.History{UsedArticleUrls: mapKeys(used)}); e != nil {
		r.Errors = append(r.Errors, fmt.Sprintf("skip: %v", e))
	} else {
		r.Actions["skip"] = true
		r.Events = append(r.Events, "skip="+string(item.Kind))
	}
	regenerated := false
	for attempt := 0; attempt < 10 && !regenerated; attempt++ {
		item, nextHist, _, e := playerSvc.NextItem(audioServer, talkSvc, musicSvc, domain.NextItemRequest{}, domain.History{UsedArticleUrls: mapKeys(used)})
		if e != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("regenerate: %v", e))
			break
		}
		used = mapFromHistory(nextHist)
		if item.Kind != domain.PlayableKindTalk {
			continue
		}
		if e = checkHTTP200(item.URL); e != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("regenerate talk: %v", e))
			break
		}
		if e = checkLoudness(audioServer.LoudnessURLForAudioURL(item.URL)); e != nil {
			r.Errors = append(r.Errors, fmt.Sprintf("regenerate loudness: %v", e))
			break
		}
		regenerated = true
		r.Actions["regenerate"] = true
		r.Events = append(r.Events, "regenerate=talk")
	}
	if !regenerated {
		r.Errors = append(r.Errors, "talk regeneration after skip was not observed")
	}
	// Structural negative cases must fail before ORT.  They are recorded by
	// name so a skipped check cannot be mistaken for a successful E2E.
	var preflightErr error
	r.Preflight, preflightErr = preflightCases(*model)
	if preflightErr != nil {
		r.Errors = append(r.Errors, preflightErr.Error())
	}
	var shutdownErr error
	r.Shutdown, shutdownErr = shutdownCheck(playerSvc, cfg)
	if shutdownErr != nil {
		r.Errors = append(r.Errors, shutdownErr.Error())
	}
	data, _ := json.MarshalIndent(r, "", "  ")
	path := filepath.Join(*out, fmt.Sprintf("e2e-%s.json", *ep))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return err
	}
	fmt.Printf("e2e report=%s cycles_ok=%d/%d errors=%d\n", path, r.CyclesOK, *cycles, len(r.Errors))
	if len(r.Errors) > 0 || r.CyclesOK != *cycles {
		return fmt.Errorf("E2E failed; see %s", path)
	}
	return nil
}

func newFixtureServer(script string) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/feed.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprintf(w, "<?xml version=\"1.0\"?><rss version=\"2.0\"><channel><title>fixture</title><link>%s</link>", r.Host)
		for i := 1; i <= 3; i++ {
			fmt.Fprintf(w, "<item><title>Fixture news %d</title><link>http://fixture.invalid/article/%d</link><description>%s</description></item>", i, i, script)
		}
		io.WriteString(w, "</channel></rss>")
	})
	mux.HandleFunc("/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": script}}}})
	})
	return httptest.NewServer(mux)
}

func mapKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	return out
}
func mapFromHistory(h domain.History) map[string]bool {
	out := map[string]bool{}
	for _, k := range h.UsedArticleUrls {
		out[k] = true
	}
	return out
}
func checkHTTP200(url string) error {
	resp, e := http.Get(url)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%s status=%d", url, resp.StatusCode)
	}
	return nil
}
func checkLoudness(url string) error {
	resp, e := http.Get(url)
	if e != nil {
		return e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("status=%d", resp.StatusCode)
	}
	var x audio.LoudnessEnvelopeResponse
	if e = json.NewDecoder(resp.Body).Decode(&x); e != nil {
		return e
	}
	if (x.SampleRate != 48000 && x.SampleRate != 44100) || len(x.RMS) == 0 {
		return fmt.Errorf("invalid envelope rate=%d rms=%d", x.SampleRate, len(x.RMS))
	}
	return nil
}

func isolatedConfigRoundTrip(dir string, cfg domain.AppConfig, v4, ep string, steps int, seconds float64) error {
	s, e := store.NewAt(dir)
	if e != nil {
		return e
	}
	if e = s.SaveConfig(cfg); e != nil {
		return e
	}
	s2, e := store.NewAt(dir)
	if e != nil {
		return e
	}
	got, e := s2.LoadConfig()
	if e != nil {
		return e
	}
	if got.Irodori.ModelDir != v4 {
		return fmt.Errorf("v4 config did not survive restart: %q", got.Irodori.ModelDir)
	}
	if err := runConfigChildProcess(dir, ep, cfg.Irodori.RefWAV, steps, seconds, v4); err != nil {
		return err
	}
	got.Irodori.ModelDir = "model/irodori-v3"
	if e = s2.SaveConfig(got); e != nil {
		return e
	}
	s3, e := store.NewAt(dir)
	if e != nil {
		return e
	}
	back, e := s3.LoadConfig()
	if e != nil {
		return e
	}
	if back.Irodori.ModelDir != "model/irodori-v3" {
		return fmt.Errorf("v3 restore failed: %q", back.Irodori.ModelDir)
	}
	if err := runConfigChildProcess(dir, ep, cfg.Irodori.RefWAV, steps, seconds, "model/irodori-v3"); err != nil {
		return err
	}
	return nil
}

func runConfigChildProcess(dir, ep, ref string, steps int, seconds float64, wantModel string) error {
	cmd := exec.Command(os.Args[0], "--config-child", "--config-dir", dir, "--ep", ep, "--ref", ref, "--steps", fmt.Sprint(steps), "--seconds", fmt.Sprint(seconds))
	cmd.Stderr = os.Stderr
	b, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("config child %s failed: %w", wantModel, err)
	}
	if !strings.Contains(string(b), "model="+wantModel) {
		return fmt.Errorf("config child selected wrong model: want %s output %q", wantModel, string(b))
	}
	return nil
}

func runConfigChild(dir, ep, ref string, steps int, seconds float64) error {
	if strings.TrimSpace(dir) == "" {
		return errors.New("config child requires --config-dir")
	}
	s, err := store.NewAt(dir)
	if err != nil {
		return err
	}
	cfg, err := s.LoadConfig()
	if err != nil {
		return err
	}
	lib := generation.ResolveORTLibraryPathForEP(ep)
	if lib == "" {
		return fmt.Errorf("matching %s ORT DLL not found", ep)
	}
	if err := generation.ConfigureExecutionProvider(ep, 0); err != nil {
		return err
	}
	if err := generation.Init(lib); err != nil {
		return err
	}
	cfg.LocalInference.ExecutionProvider, cfg.LocalInference.ORTLibraryPath = ep, lib
	if strings.TrimSpace(cfg.Irodori.RefWAV) == "" {
		cfg.Irodori.RefWAV = ref
	}
	cfg.Irodori.Seconds, cfg.Irodori.NumSteps = seconds, steps
	tmp, err := os.MkdirTemp("", "fm-radio-config-child-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	wav, err := localtts.New().SynthesizeWav(context.Background(), cfg, "設定再起動子プロセスの短いTalkです。")
	if err != nil {
		return err
	}
	if len(wav) == 0 {
		return errors.New("config child produced empty WAV")
	}
	fmt.Printf("config child model=%s bytes=%d\n", cfg.Irodori.ModelDir, len(wav))
	return nil
}

func preflightCases(model string) (map[string]string, error) {
	out := map[string]string{}
	root, err := os.MkdirTemp("", "fm-radio-preflight-")
	if err != nil {
		out["setup"] = "FAILED: " + err.Error()
		return out, err
	}
	defer os.RemoveAll(root)
	bad := filepath.Join(root, "unknown-schema")
	if err := os.MkdirAll(bad, 0o755); err != nil {
		return out, fmt.Errorf("schema fixture mkdir: %w", err)
	}
	m, err := os.ReadFile(filepath.Join(model, "manifest.json"))
	if err != nil {
		return out, fmt.Errorf("schema fixture read: %w", err)
	}
	m = bytesReplace(m, []byte(`"schema_version": 2`), []byte(`"schema_version": 999`))
	if err := os.WriteFile(filepath.Join(bad, "manifest.json"), m, 0o600); err != nil {
		return out, fmt.Errorf("schema fixture write: %w", err)
	}
	defer os.RemoveAll(bad)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err = localtts.New().SynthesizeWav(ctx, domain.AppConfig{Irodori: domain.IrodoriConfig{ModelDir: bad}}, "テスト。")
	if err != nil {
		if !strings.Contains(err.Error(), "unsupported schema_version 999 (want 2)") {
			return out, fmt.Errorf("schema999 unexpected error: %w", err)
		}
		out["unknown_schema"] = "rejected: " + err.Error()
	} else {
		out["unknown_schema"] = "FAILED: accepted"
		return out, errors.New("schema999 negative case was accepted")
	}
	missing := filepath.Join(root, "missing-model")
	_, err = localtts.New().SynthesizeWav(ctx, domain.AppConfig{Irodori: domain.IrodoriConfig{ModelDir: missing}}, "テスト。")
	if err != nil {
		out["missing_bundle"] = "rejected: " + err.Error()
	} else {
		out["missing_bundle"] = "FAILED: accepted"
		return out, errors.New("missing bundle negative case was accepted")
	}
	return out, nil
}
func bytesReplace(b, old, new []byte) []byte {
	if i := strings.Index(string(b), string(old)); i >= 0 {
		out := append([]byte{}, b[:i]...)
		out = append(out, new...)
		out = append(out, b[i+len(old):]...)
		return out
	}
	return b
}

func hashBytes(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func hashFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "unavailable:" + err.Error()
	}
	return hashBytes(b)
}
func hashScripts(s []ttseval.Script) string { b, _ := json.Marshal(s); return hashBytes(b) }
func vcsIDs() (string, string) {
	read := func(rev string) string {
		b, err := exec.Command("jj", "log", "-r", rev, "-T", "commit_id", "--no-graph").CombinedOutput()
		v := strings.TrimSpace(string(b))
		if err != nil {
			return "unavailable:" + v
		}
		if !regexp.MustCompile(`^[0-9a-fA-F]{40}$`).MatchString(v) {
			return "unavailable:jj commit id is not 40 hex"
		}
		return strings.ToLower(v)
	}
	return read("@"), read("@-")
}
func hashModelAssets(root string) map[string]string {
	out := map[string]string{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		base, ext := strings.ToLower(filepath.Base(path)), strings.ToLower(filepath.Ext(path))
		if base == "manifest.json" || base == "metadata.json" || base == "tokenizer.json" || ext == ".onnx" || ext == ".data" {
			rel, _ := filepath.Rel(root, path)
			out[filepath.ToSlash(rel)] = hashFile(path)
		}
		return nil
	})
	return out
}

func shutdownCheck(p *player.Player, cfg domain.AppConfig) (map[string]any, error) {
	_ = p // the caller's Player is also shut down by its defer; use fresh cases below.
	// A ready flag is not evidence of active inference. Each case waits for the
	// real runtime start event, then calls Player.Shutdown and only afterwards
	// destroys the shared ORT environment.
	type shutdownCase struct {
		name string
		cfg  domain.AppConfig
	}
	cases := []shutdownCase{
		{name: "talk", cfg: cfg},
		{name: "stable_audio_bgm", cfg: cfg},
		{name: "talk_and_bgm_prefetch", cfg: cfg},
	}
	results := make([]map[string]any, 0, len(cases))
	var failures []string
	for _, tc := range cases {
		caseCfg := tc.cfg
		caseCfg.Talk.CycleBgmCount = 1 // makes Talk prefetch eligible immediately
		talkSvc := talk.New(filepath.Join(os.TempDir(), "fm-radio-shutdown-", tc.name))
		musicSvc := musicgen.New()
		events := make(chan string, 32)
		// Hold each runtime immediately after its real start event. This keeps
		// every requested provider in-flight until the main goroutine has
		// observed all starts and is ready to invoke Player.Shutdown.
		releaseInference := make(chan struct{})
		eventTimes := map[string][]time.Time{}
		var eventMu sync.Mutex
		talkSvc.SetObserver(func(e irodoriPipeline.RuntimeEvent) {
			key := "talk:" + string(e)
			eventMu.Lock()
			eventTimes[key] = append(eventTimes[key], time.Now())
			eventMu.Unlock()
			events <- key
			if e == irodoriPipeline.EventInferenceStart {
				<-releaseInference
			}
		})
		musicSvc.SetObserver(func(e stablepipeline.RuntimeEvent) {
			key := "bgm:" + string(e)
			eventMu.Lock()
			eventTimes[key] = append(eventTimes[key], time.Now())
			eventMu.Unlock()
			events <- key
			if e == stablepipeline.EventInferenceStart {
				<-releaseInference
			}
		})
		cp := player.New(caseCfg)
		if tc.name == "talk" || tc.name == "talk_and_bgm_prefetch" {
			cp.PrefetchTalk(talkSvc, caseCfg, domain.History{})
		}
		if tc.name == "stable_audio_bgm" || tc.name == "talk_and_bgm_prefetch" {
			cp.PrefetchMusic(musicSvc, caseCfg)
		}
		want := map[string]bool{"talk": false, "bgm": false}
		if tc.name == "talk" || tc.name == "talk_and_bgm_prefetch" {
			want["talk"] = true
		}
		if tc.name == "stable_audio_bgm" || tc.name == "talk_and_bgm_prefetch" {
			want["bgm"] = true
		}
		started := map[string]bool{}
		lifecycle := make([]string, 0, 16)
		// CPU short runs can spend well over ten seconds between reservation and
		// the first ORT call. Never treat a ready/active flag as a substitute for
		// the real start barrier; wait long enough for the event or fail closed.
		deadline := time.NewTimer(120 * time.Second)
		for (want["talk"] && !started["talk"]) || (want["bgm"] && !started["bgm"]) {
			select {
			case e := <-events:
				lifecycle = append(lifecycle, e)
				if strings.HasPrefix(e, "talk:") && strings.HasSuffix(e, ":"+string(irodoriPipeline.EventInferenceStart)) {
					started["talk"] = true
				}
				if strings.HasPrefix(e, "bgm:") && strings.HasSuffix(e, ":"+string(stablepipeline.EventInferenceStart)) {
					started["bgm"] = true
				}
			case <-deadline.C:
				started["timeout"] = true
			}
			if started["timeout"] {
				break
			}
			if (!want["talk"] || started["talk"]) && (!want["bgm"] || started["bgm"]) {
				break
			}
		}
		if !deadline.Stop() {
			select {
			case <-deadline.C:
			default:
			}
		}
		before := cp.Status()
		activeBefore := before.TalkPrefetching || before.MusicGenerating
		if !activeBefore {
			failures = append(failures, tc.name+" shutdown barrier was not observed while inference was active")
		}
		// Shutdown is the cancellation request boundary. Start it while each
		// real inference observer is still blocked, wait for Player's accepted
		// cancellation signal, then release ORT and wait for the worker/service.
		shutdownDone := make(chan struct{})
		go func() {
			cp.Shutdown()
			close(shutdownDone)
		}()
		shutdownAccepted := false
		select {
		case <-cp.ShutdownAccepted():
			shutdownAccepted = true
		case <-time.After(5 * time.Second):
			failures = append(failures, tc.name+" shutdown acceptance timeout")
		}
		shutdownAt := time.Now()
		if shutdownAccepted {
			for kind, required := range want {
				if required {
					key := kind + ":shutdown accepted"
					eventMu.Lock()
					eventTimes[key] = append(eventTimes[key], shutdownAt)
					eventMu.Unlock()
					lifecycle = append(lifecycle, key)
				}
			}
		}
		close(releaseInference)
		<-shutdownDone
		after := cp.Status()
		// Player.Shutdown waits for the service worker, and the service worker
		// emits close end only after Runtime.Close has completed. Waiting for each
		// required close-end event drains the exact lifecycle barrier without a
		// timing sleep that could hide a late join/publication.
		var drainErr error
		lifecycle, drainErr = drainLifecycleEvents(events, lifecycle, want, 5*time.Second)
		if drainErr != nil {
			failures = append(failures, tc.name+" lifecycle drain: "+drainErr.Error())
		}
		got := map[string]any{"inference_start": started, "cancel_requested": true, "shutdown_accepted": shutdownAccepted, "player_joined": drainErr == nil, "lifecycle_events": lifecycle, "status_before": before, "status_after": after, "active_before": activeBefore, "active_shutdown": !after.TalkPrefetching && !after.MusicGenerating && !after.TalkReady && !after.MusicReady, "stale_ready_or_error": after.TalkReady || after.MusicReady || after.LocalGenerationError != ""}
		results = append(results, map[string]any{"case": tc.name, "result": got})
		for k, required := range want {
			if required && !started[k] {
				failures = append(failures, tc.name+" missing real "+k+" inference-start")
			}
		}
		if !got["active_shutdown"].(bool) || got["stale_ready_or_error"].(bool) {
			failures = append(failures, tc.name+" stale active/ready/error after join")
		}
		for kind, required := range want {
			if !required {
				continue
			}
			prefix := kind + ":"
			seen := map[string]bool{}
			for _, event := range lifecycle {
				if strings.HasPrefix(event, prefix) {
					seen[strings.TrimPrefix(event, prefix)] = true
				}
			}
			for _, event := range []string{string(irodoriPipeline.EventInferenceJoined), string(irodoriPipeline.EventServiceJoined), string(irodoriPipeline.EventCloseStart), string(irodoriPipeline.EventCloseEnd)} {
				if kind == "bgm" && event == string(irodoriPipeline.EventInferenceJoined) {
					// Stable Audio emits the same join event, but use its type
					// below so the report is explicit about the provider.
					continue
				}
				if kind == "talk" && event == string(stablepipeline.EventInferenceJoined) {
					continue
				}
				if !seen[event] {
					failures = append(failures, tc.name+" missing "+kind+" lifecycle event "+event)
				}
			}
			joinEvent := string(irodoriPipeline.EventInferenceJoined)
			serviceJoinEvent := string(irodoriPipeline.EventServiceJoined)
			if kind == "bgm" {
				joinEvent = string(stablepipeline.EventInferenceJoined)
				serviceJoinEvent = string(stablepipeline.EventServiceJoined)
			}
			if !seen[joinEvent] {
				failures = append(failures, tc.name+" missing "+kind+" lifecycle event "+joinEvent)
			}
			if !seen[serviceJoinEvent] {
				failures = append(failures, tc.name+" missing "+kind+" service lifecycle event "+serviceJoinEvent)
			}
			// The event trace must prove the cancellation/shutdown was accepted
			// while inference was active, and that service join (not merely the
			// pipeline's own join event) preceded Runtime.Close. Sequence indexes
			// are the authoritative order because callbacks are emitted at the
			// exact lifecycle boundary; timestamps are retained as a diagnostic.
			startIndex := indexOfEvent(lifecycle, prefix+string(irodoriPipeline.EventInferenceStart))
			shutdownIndex := indexOfEvent(lifecycle, prefix+"shutdown accepted")
			endIndex := indexOfEvent(lifecycle, prefix+string(irodoriPipeline.EventInferenceEnd))
			serviceIndex := indexOfEvent(lifecycle, prefix+serviceJoinEvent)
			closeStartIndex := indexOfEvent(lifecycle, prefix+string(irodoriPipeline.EventCloseStart))
			closeEndIndex := indexOfEvent(lifecycle, prefix+string(irodoriPipeline.EventCloseEnd))
			if kind == "bgm" {
				startIndex = indexOfEvent(lifecycle, prefix+string(stablepipeline.EventInferenceStart))
				endIndex = indexOfEvent(lifecycle, prefix+string(stablepipeline.EventInferenceEnd))
				closeStartIndex = indexOfEvent(lifecycle, prefix+string(stablepipeline.EventCloseStart))
				closeEndIndex = indexOfEvent(lifecycle, prefix+string(stablepipeline.EventCloseEnd))
			}
			if startIndex < 0 || shutdownIndex < 0 || endIndex < 0 || serviceIndex < 0 || closeStartIndex < 0 || closeEndIndex < 0 || !(startIndex < shutdownIndex && shutdownIndex < endIndex && endIndex < serviceIndex && serviceIndex < closeStartIndex && closeStartIndex < closeEndIndex) {
				failures = append(failures, fmt.Sprintf("%s invalid lifecycle sequence start < shutdown < inference end < service join < close (%s)", tc.name, kind))
			}
		}
	}
	// The main Player may still own a prefetch started during the cycle run;
	// join it before touching the shared ORT environment as well.
	p.Shutdown()
	shutdownErr := generation.Shutdown()
	result := map[string]any{"cases": results, "generation_shutdown": shutdownErr == nil, "shutdown_order": []string{"Player.Shutdown", "join", "generation.Shutdown"}, "reservation_flags_not_used_as_barrier": true}
	if shutdownErr != nil {
		failures = append(failures, "generation.Shutdown: "+shutdownErr.Error())
	}
	if len(failures) > 0 {
		result["errors"] = failures
		return result, errors.New(strings.Join(failures, "; "))
	}
	return result, nil
}

func indexOfEvent(events []string, want string) int {
	for i, event := range events {
		if event == want {
			return i
		}
	}
	return -1
}

func drainLifecycleEvents(events <-chan string, lifecycle []string, want map[string]bool, timeout time.Duration) ([]string, error) {
	required := make(map[string]bool)
	if want["talk"] {
		required["talk:"+string(irodoriPipeline.EventCloseEnd)] = true
	}
	if want["bgm"] {
		required["bgm:"+string(stablepipeline.EventCloseEnd)] = true
	}
	seen := make(map[string]bool, len(required))
	for _, event := range lifecycle {
		if required[event] {
			seen[event] = true
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for len(seen) < len(required) {
		select {
		case event := <-events:
			lifecycle = append(lifecycle, event)
			if required[event] {
				seen[event] = true
			}
		case <-timer.C:
			missing := make([]string, 0, len(required)-len(seen))
			for event := range required {
				if !seen[event] {
					missing = append(missing, event)
				}
			}
			return lifecycle, fmt.Errorf("missing close-end barrier events: %s", strings.Join(missing, ", "))
		}
	}
	// All service/runtime work has reached close-end. Drain already queued
	// callbacks without waiting for an arbitrary grace period.
	for {
		select {
		case event := <-events:
			lifecycle = append(lifecycle, event)
		default:
			return lifecycle, nil
		}
	}
}
