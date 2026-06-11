# Local Generation Research Notes

## Scope

Reusable implementation facts for integrating local Stable Audio 3 music generation and IrodoriTTS speech generation into `fm-live-radio`.

Confirmed from local repositories on 2026-06-11:

- `E:\programming\AI_generative\VibeCoding\stuble-audio-3-research`
- `E:\programming\AI_generative\VibeCoding\tts-research`

This cheatsheet summarizes local research artifacts. It does not revalidate upstream Web sources.

## Stable Audio 3 Go Research

### Target

- Target model: Stable Audio 3 small-music.
- Runtime: ONNX Runtime 1.26.0, CPU EP.
- Output: 44.1 kHz stereo WAV.
- Go module in research repo: `github.com/rero2/stable-audio-3`.

### Environment

- Go 1.25.4+ in the research README.
- CGO required.
- Windows needs MSYS2 UCRT64 `gcc.exe`.
- ORT DLL path can be supplied through `SA3_ORT_LIB` in the research repo.

### Required Model Layout

```text
models/sa3-sm-music/
  tokenizer/tokenizer.json
  onnx/t5gemma/encoder.onnx
  onnx/sa3-sm-music/dit_fp16mixed.onnx
  onnx/same-s/dec_dynamic_bf16.onnx
```

### ONNX IO Facts From Research Report

T5Gemma encoder:

- inputs: `input_ids` int64 `(1, 256)`, `attention_mask` int64 `(1, 256)`
- output: `hidden_states` float32 `(1, 256, 768)`

DiT:

- inputs:
  - `x` float32 `(1, 256, L)`
  - `t` float32 `(1,)`
  - `t5_hidden` float32 `(1, 256, 768)`
  - `t5_mask` float32 `(1, 256)`
  - `seconds_total` float32 `(1,)`
  - `local_add_cond` float32 `(1, 257, L)`
- output: `velocity` float32 `(1, 256, L)`
- Research smoke test says `dit_fp16mixed.onnx` ran on ORT CPU EP.

SAME-S decoder:

- input: `latent` float32 `(1, 256, L)`
- output: `pcm` int32, observed as `(1, T_full, 2)`
- Research smoke test says `dec_dynamic_bf16.onnx` ran on ORT CPU EP.

### Pipeline Options

Research `pipeline.Options`:

```go
type Options struct {
    Prompt     string
    Seconds    float64
    Steps      int
    Seed       uint32
    ModelDir   string
    OutputWAV  string
    SampleRate int
}
```

Defaults:

- seconds: 30
- steps: 8
- sample rate: 44100

### Integration Notes

- Research code lives under `internal`, so direct import from another module is not possible without refactoring.
- `ort.Init` should be unified with IrodoriTTS integration instead of duplicated.
- `local_add_cond` is zero-filled for text-to-audio in the current research pipeline.
- Generated output should be validated for valid WAV header and non-zero peak/RMS during integration.

## IrodoriTTS Go Research

### Target

- Japanese text-to-WAV local TTS.
- `fm-live-radio` integration target is IrodoriTTS v3.
- Runtime: ONNX Runtime through Go.
- Output: 48 kHz mono WAV for the documented v2/v3 pipeline.
- Go module in research repo: `github.com/rero2/irodori-tts`.

### Environment

- Go 1.25+ in README.
- Windows + CGO + MSYS2 UCRT64 gcc.
- ORT DLL path can be supplied through `IRODORI_ORT_LIB` in the research repo.
- README notes v3 model loading can require 2.5 GB+ free memory.

### Model Layout

v2 caption mode:

```text
models/v2-voicedesign/
  metadata.json
  tokenizer.json
  caption_tokenizer.json
  text_encoder.onnx
  caption_encoder.onnx
  dit_step.onnx
  dacvae_encoder.onnx
  dacvae_decoder.onnx
```

v3 speaker mode:

```text
models/v3/
  metadata.json
  tokenizer.json
  text_encoder.onnx
  speaker_encoder.onnx
  duration_predictor.onnx
  dacvae_encoder.onnx
  dacvae_decoder.onnx
```

### Pipeline Options

Research `pipeline.Options`:

```go
type Options struct {
    Text          string
    Caption       string
    OutputWAV     string
    ModelDir      string
    Seed          uint32
    NumSteps      int
    Seconds       float64
    CfgText       float64
    CfgCaption    float64
    RefWAV        string
    CfgSpeaker    float64
    DurationScale float64
}
```

Defaults:

- num steps: 40
- seconds: -1.0
- cfg text: 3.0
- cfg caption: 3.0
- cfg speaker: 5.0
- duration scale: 1.0

### Integration Notes

- The research CLI calls `ort.Init("")`, loads `pipeline.Runtime`, runs `Synthesize`, and writes WAV to `OutputWAV`.
- Research code lives under `internal`, so direct import from another module is not possible without refactoring.
- `fm-live-radio` should use v3. If `narrator` contains WAV files, pass the first listed WAV as `RefWAV`; if no WAV exists, run the v3 default speaker path.
- Current README states v2 and v3 support, while source comments still mention v2 MVP in places. Inspect current code and smoke-test v3 in the target app before finalizing UI claims.

## Shared Integration Rules For fm-live-radio

- Use one shared ORT initializer in `fm-live-radio`.
- Do not commit models or ONNX Runtime binary bundles unless explicitly approved.
- Keep `files` BGM and `gemini` TTS as compatibility providers during migration.
- User-facing name is `Stable Audio 3`; `Stuble Audio 3` was a typo in the original request. The local research path still contains `stuble-audio-3-research`.
- Default local directories:
  - `<base>/model` for model assets.
  - `<base>/narrator` for IrodoriTTS v3 reference voice WAV files.
  - `<base>/generate_music` for Stable Audio 3 generated music cache.
- `<base>` should be the packaged exe current directory or, in development, the project root.
- `generate_music` should keep about 20 generated music WAV files and delete older files first.
- If Stable Audio 3 generation is late and `generate_music` has cached files, choose a cached file for BGM fallback.
- Cache fallback selection should sort generated files by oldness and choose around index `n/2`. Avoid selecting the oldest file because it is likely to be deleted when a newly generated file enters the cache.
- Run local smoke tests after copying or refactoring providers into `fm-live-radio`.
- Record any divergence from research code back into this cheatsheet.

## Open Questions

- Whether to refactor research repos into importable packages or copy/adapt implementation into `fm-live-radio`.
- Exact runtime detection rule for packaged exe directory versus project root.

## Update Conditions

Update this cheatsheet when:

- ONNX Runtime version changes.
- Go version changes.
- Model layouts differ from the research repos.
- Integration smoke tests in `fm-live-radio` produce different output names, shapes, sample rates, or performance.
