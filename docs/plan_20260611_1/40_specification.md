# Implementation Specification

## Technical Stack

- Desktop: Wails v2
- Backend: Go
- Frontend: React + Vite + TypeScript
- Local inference: ONNX Runtime 1.26.0 through `github.com/yalue/onnxruntime_go`
- Music generation: Stable Audio 3 small-music Go pipeline from local research repository
- Speech generation: IrodoriTTS Go pipeline from local research repository
- Tooling: `mise`, `jj`

## File Structure

Proposed additions and changes:

```text
internal/
  generation/
    ort_runtime.go              # shared ORT init/shutdown and path resolution
    queue.go                    # single-worker generation queue and cancellation
    errors.go
  musicgen/
    service.go                  # BGM generation facade used by player
    prompt.go                   # prompt construction
    cache.go                    # generate_music retention and fallback selection
    stableaudio/
      ...                       # imported/adapted SA3 packages
  localtts/
    service.go                  # TTS facade used by talk.Service
    irodori/
      ...                       # imported/adapted IrodoriTTS packages
  tts/
    gemini_tts.go               # keep existing provider
    provider.go                 # common TTSProvider interface
  talk/
    talk.go                     # route through TTSProvider
  player/
    player.go                   # route BGM slot to files or musicgen
  domain/
    types.go                    # config/status/playable source expansion
docs/
  cheatsheet/
    local-generation-research.md
```

The exact package layout may be adjusted during implementation, but the design goal is to keep local inference engines behind small facades so `player` and `talk` do not depend on model internals.

## Data Model

### Config

Add fields to `domain.AppConfig`.

```go
type BGMSource string
const (
    BGMSourceFiles BGMSource = "files"
    BGMSourceStableAudio3 BGMSource = "stable_audio_3"
)

type TTSSource string
const (
    TTSSourceGemini TTSSource = "gemini"
    TTSSourceIrodori TTSSource = "irodori"
)

type LocalInferenceConfig struct {
    ORTLibraryPath string `json:"ortLibraryPath"`
    MaxWorkers     int    `json:"maxWorkers"`
}

type StableAudio3Config struct {
    Enabled       bool    `json:"enabled"`
    ModelDir      string  `json:"modelDir"`
    OutputDir     string  `json:"outputDir"`
    PromptBase    string  `json:"promptBase"`
    Seconds       float64 `json:"seconds"`
    Steps         int     `json:"steps"`
    SeedMode      string  `json:"seedMode"` // random, fixed, sequential
    FixedSeed     uint32  `json:"fixedSeed"`
    CacheLimit    int     `json:"cacheLimit"`
}

type IrodoriConfig struct {
    Enabled       bool    `json:"enabled"`
    ModelDir      string  `json:"modelDir"`
    NarratorDir   string  `json:"narratorDir"`
    RefWAV        string  `json:"refWav"`
    Seconds       float64 `json:"seconds"`
    NumSteps      int     `json:"numSteps"`
    SeedMode      string  `json:"seedMode"`
    FixedSeed     uint32  `json:"fixedSeed"`
    CfgText       float64 `json:"cfgText"`
    CfgCaption    float64 `json:"cfgCaption"`
    CfgSpeaker    float64 `json:"cfgSpeaker"`
    DurationScale float64 `json:"durationScale"`
}
```

Existing `TTSConfig` can either be extended with `Provider` or kept for Gemini-specific fields while new provider config is added. Implementation should prefer backward-compatible defaults:

- `bgmSource` default: `files`
- `ttsSource` default: `gemini`
- `localInference.maxWorkers` default: `1`
- `stableAudio3.modelDir` default: `<base>/model/sa3-sm-music`
- `stableAudio3.outputDir` default: `<base>/generate_music`
- `stableAudio3.seconds` default: `30`
- `stableAudio3.steps` default: `8`
- `stableAudio3.cacheLimit` default: `20`
- `irodori.modelDir` default: `<base>/model/irodori-v3`
- `irodori.narratorDir` default: `<base>/narrator`
- `irodori.seconds` default: `-1`
- `irodori.numSteps` default: `40`

`<base>` is resolved as the exe current directory in packaged execution and the project root in development execution.

### PlayableItem Source

Extend `PlayableSource` with optional debug fields.

```go
type PlayableSource struct {
    Genre      string `json:"genre,omitempty"`
    FilePath   string `json:"filePath,omitempty"`
    RssURL     string `json:"rssUrl,omitempty"`
    ArticleURL string `json:"articleUrl,omitempty"`
    Provider   string `json:"provider,omitempty"`
    Prompt     string `json:"prompt,omitempty"`
    Seed       uint32 `json:"seed,omitempty"`
    ModelDir   string `json:"modelDir,omitempty"`
}
```

## Backend Design

### Shared ORT Runtime

- Add one process-level ORT initializer.
- Initialize lazily when local generation provider is first used.
- Resolve ORT DLL path in this order:
  1. config `localInference.ortLibraryPath`
  2. `FM_RADIO_ORT_LIB`
  3. bundled or user-provided default path under app config if present
  4. provider-specific env var only as compatibility fallback
- Shutdown during Wails app shutdown.

Important: Do not call separate `ort.Init` implementations from SA3 and Irodori packages if they each own global ORT state. Adapt them to use a common wrapper or copy the common `internal/ort` implementation once.

### Music Generation Service

Interface:

```go
type MusicGenerator interface {
    Generate(ctx context.Context, cfg domain.AppConfig, req MusicRequest) (MusicResult, error)
}

type MusicRequest struct {
    Mood        string
    PromptHint  string
    DurationSec float64
}

type MusicResult struct {
    AudioPath string
    Title     string
    Prompt    string
    Seed      uint32
}
```

Stable Audio 3 implementation:

- Load tokenizer and ONNX sessions from `stableAudio3.modelDir`.
- Reuse runtime across generations while modelDir and settings are unchanged.
- Generate WAV under `stableAudio3.outputDir`, defaulting to `<base>/generate_music`.
- Keep about `stableAudio3.cacheLimit` files, default 20; remove older generated music first.
- If generation is not ready by playback time, select a playable cache entry from `generate_music`.
- Cache fallback selection sorts generated files by modified time ascending, then chooses the file around index `len(files)/2`. This avoids selecting the oldest file, which may become the next deletion target while playback is using it.
- Trim to requested seconds as the research pipeline already does.
- Return error on model missing, ORT init failure, generation failure.

### TTS Provider

Introduce:

```go
type Provider interface {
    SynthesizeWav(ctx context.Context, cfg domain.AppConfig, text string) ([]byte, error)
}
```

- Gemini provider wraps current `tts.GeminiClient`.
- Irodori provider wraps IrodoriTTS v3 pipeline and writes to a temp path or returns WAV bytes.
- Irodori provider chooses `RefWAV` from config when set; otherwise it scans `irodori.narratorDir` for WAV files and uses the first file returned by the file listing. If no WAV exists, it runs v3 default speaker mode without reference WAV.
- `talk.Service.Generate` selects provider from config and stays responsible for RSS, LLM, and final temp file writing.

Irodori provider sentence synthesis:

- Keep Gemini as a single-shot provider because it performs better with full-script context.
- Implement sentence splitting inside the Irodori provider path, not in `talk.Service.Generate`, so Gemini remains unchanged.
- Split text on Japanese and ASCII sentence terminators such as `。`, `！`, `？`, `!`, `?`, and line breaks.
- Trim whitespace and drop empty segments.
- Synthesize each sentence independently through the existing Irodori pipeline.
- Decode generated WAV payloads as PCM16 WAV and concatenate PCM data into a single 48 kHz mono WAV.
- Insert a short silence gap between successful sentences. MVP gap: 300 ms.
- If an individual sentence fails to synthesize, insert 3 seconds of 48 kHz mono PCM16 silence in that sentence slot and continue with the remaining sentences.
- If all sentences are empty before synthesis, return an error.
- If every sentence fails and the final output would be only fallback silence, still return the combined WAV for MVP so playback continues and the failure is audible as a gap rather than aborting the radio cycle.
- Add small WAV helper functions under `internal/audiofmt` or `internal/localtts` for:
  - parsing PCM16 WAV payload metadata and data
  - validating matching sample rate, channel count, and bit depth
  - generating PCM16 silence
  - re-encoding combined PCM via existing `audiofmt.EncodeWavPCM16`

### LLM Talk Script Generation

- `internal/llm.OpenAICompat` keeps using `/chat/completions`.
- Set the chat completion output limit to `max_tokens: 8192`.
  - `ollama show gemma4:12b` reports model context length `262144`.
  - `ollama ps` reports the currently loaded runtime context as `32768`.
  - The app's news prompt is small enough that `8192` is safe within the current runtime context while preventing thinking-capable models from consuming the previous `400` token budget before emitting `message.content`.
- `talk.Service.Generate` must validate the LLM script before invoking TTS.
  - If `strings.TrimSpace(script) == ""`, return an error and do not call `provider.SynthesizeWav`.
  - This avoids generating a short 1-second Irodori WAV from an empty prompt.

### Player Integration

Current `player.pickBGM` becomes provider-aware.

- If `cfg.BGMSource == files`, use existing `bgm.ListTracks`.
- If `cfg.BGMSource == stable_audio_3`, request `musicgen.Service.Generate`.
- Increment `bgmCountSinceLastTalk` after a BGM item is successfully selected or generated.
- On Stable Audio 3 failure:
  - If `generate_music` has cached files, select the old-order midpoint item as fallback.
  - Otherwise return a typed error for UI display or short silence retry.

### Generation Queue

MVP can keep player-level prefetch with a single in-flight job, but heavy local generation needs shared throttling.

- `generation.Queue` accepts jobs with context cancellation.
- Default worker count is 1.
- BGM prefetch and Talk prefetch use the same queue.
- Skip / stop / settings save cancels queued or in-flight jobs when possible.
- AppStatus reports:
  - `musicGenerating`
  - `musicReady`
  - `talkPrefetching`
  - `talkReady`
  - `localGenerationError`

## UI / Screens

Settings additions:

- BGM Source segmented control: Files / Stable Audio 3
- TTS Provider segmented control: Gemini / IrodoriTTS
- ORT DLL path input
- Stable Audio 3 section:
  - model directory
  - generate_music directory
  - prompt base
  - seconds
  - steps
  - seed mode
  - cache limit
- IrodoriTTS section:
  - model directory
  - narrator directory
  - reference WAV path
  - seconds / auto
  - num steps
  - CFG values
  - duration scale

Player additions:

- Show provider name in current source detail when local generation is active.
- Existing generation lamp should distinguish Music generating and Talk generating.
- Show concise configuration errors when local generation cannot start.

## Inputs

- RSS URLs remain user-provided.
- LLM settings remain OpenAI-compatible.
- Music prompt is built from:
  - `stableAudio3.promptBase`
  - selected genre or mood if present
  - optional radio context such as "instrumental, no vocals, seamless background music"
- TTS text is LLM generated script.

## Persistence

- `config.json` gains local generation fields with default migration.
- `history.json` remains for article URLs.
- generated files:
  - Talk: existing `temp_audio/talk_*.wav`
  - Music: `<base>/generate_music/music_*.wav`
- On app startup, clean old Talk temp audio.
- Keep Music cache bounded to about `stableAudio3.cacheLimit` files, default 20, and remove older generated music first.
- Cache deletion must not delete the currently registered or currently playing cache fallback file. If exact playback ownership is hard to determine, choosing the `n/2` fallback item and deleting strictly oldest files is the MVP mitigation.

## Error Handling

- Typed errors for:
  - ORT not configured
  - model directory missing required files
  - CGO / DLL load failure
  - generation timeout
  - generated WAV missing or silent
- Empty LLM talk script must be treated as a Talk generation error before TTS synthesis.
- Irodori per-sentence synthesis failure is handled locally in the Irodori provider by inserting 3 seconds of silence; it does not fail the whole Talk generation unless setup fails before any sentence processing can begin.
- Provider errors must not expose API keys.
- Talk generation failure consumes the Talk slot as current behavior does.
- Music generation failure falls back to file BGM if enabled; otherwise stops playback with actionable UI error.

## Import / Export

- No model download or export in MVP.
- The app expects model directories prepared externally under `<base>/model` by default.
- Documentation must explain required directory structures.

## Environment Constraints

- Windows x64 first.
- ONNX Runtime 1.26.0.
- Stable Audio 3 model path defaults to `<base>/model/sa3-sm-music` and must contain:

```text
model/sa3-sm-music/
  tokenizer/tokenizer.json
  onnx/t5gemma/encoder.onnx
  onnx/sa3-sm-music/dit_fp16mixed.onnx
  onnx/same-s/dec_dynamic_bf16.onnx
```

- IrodoriTTS v3 model path defaults to `<base>/model/irodori-v3` and must contain v3 model assets as documented in `tts-research/README.md`.
- Narrator WAV files default to `<base>/narrator/*.wav`.
- Generated music cache defaults to `<base>/generate_music/*.wav`.

## Verification

Use `mise` from `E:\programming\AI_generative\fm-live-radio`.

Pre-implementation checks:

```powershell
jj status
mise install
mise x -- go version
mise x -- go test ./...
```

After Go API changes:

```powershell
mise x -- wails generate module
```

Backend verification:

```powershell
mise x -- go test ./...
```

Build verification:

```powershell
mise run build
```

Manual smoke checks:

- Stable Audio 3: generate a short WAV, confirm file exists, WAV header is valid, peak/RMS is non-zero.
- IrodoriTTS v3: generate a short Japanese WAV, confirm file exists, sample rate/channels match expected provider output, peak/RMS is non-zero.
- App: run `mise run dev`, switch to `stable_audio_3 + irodori`, play through at least one `BGM -> Talk -> BGM` cycle.

## Implementation Order

1. Add current-doc baseline note or keep this plan as the pre-implementation source of truth until implementation starts.
2. Add shared ORT runtime and local generation config types with default migration tests.
3. Integrate IrodoriTTS behind a TTS provider interface while keeping Gemini as default.
4. Integrate Stable Audio 3 behind a music generator interface while keeping file BGM as default.
5. Add generation queue/status and cancellation behavior.
6. Update frontend settings and status display.
7. Run Wails binding generation and verification.
8. Update current docs after implementation matches behavior.
