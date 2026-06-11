# Review Checklist

## Security

- [ ] API keys remain hidden in logs and UI error details.
- [ ] Local model paths and generated file paths are not exposed unnecessarily.
- [ ] RSS article text and LLM prompts are not logged at high verbosity by default.
- [ ] Local audio server still restricts access to registered files only.

## Frontend

- [ ] Settings can select BGM provider and TTS provider independently.
- [ ] Local generation settings are visible only when relevant providers are selected.
- [ ] Generation status distinguishes music generation and talk generation.
- [ ] Configuration errors are actionable and do not overflow compact UI areas.
- [ ] Existing files + Gemini workflow remains usable.

## Backend

- [ ] Stable Audio 3 and IrodoriTTS use a shared ORT lifecycle.
- [ ] Provider interfaces keep `player` and `talk` independent from model internals.
- [ ] Generation queue limits concurrent local inference.
- [ ] Stop, skip, and settings save cancel or invalidate obsolete jobs.
- [ ] Typed errors distinguish setup failure, model validation failure, timeout, and synthesis failure.

## DB / Storage

- [ ] `config.json` migration preserves existing configs.
- [ ] `history.json` behavior remains unchanged.
- [ ] Stable Audio 3 music is stored under `generate_music`.
- [ ] `generate_music` is bounded to about 20 cached files and removes older files first.
- [ ] Talk WAV temp files are cleaned up or bounded.
- [ ] Model files and ORT DLLs are not committed.
- [ ] `model`, `narrator`, and `generate_music` directories are gitignored or otherwise kept out of commits when they contain large/generated assets.

## QA / Test

- [ ] `mise x -- go test ./...` passes.
- [ ] `mise x -- wails generate module` succeeds after API changes.
- [ ] `mise run build` succeeds.
- [ ] Stable Audio 3 integration creates a valid non-silent WAV.
- [ ] IrodoriTTS v3 integration creates a valid non-silent WAV.
- [ ] IrodoriTTS v3 falls back to default speaker when `narrator` has no WAV.
- [ ] IrodoriTTS v3 uses the first listed WAV when `narrator` has multiple WAV files.
- [ ] Stable Audio 3 late-generation path plays from `generate_music` cache when available.
- [ ] Stable Audio 3 cache fallback chooses around the old-order midpoint rather than the oldest file.
- [ ] Manual playback covers `files+gemini`, `files+irodori`, `stable_audio_3+gemini`, `stable_audio_3+irodori`.
- [ ] Missing model directory and missing ORT DLL produce user-visible errors.

## DevOps / Environment

- [ ] `mise.toml` and `go.mod` agree on required Go version.
- [ ] CGO / MSYS2 UCRT64 gcc requirement is documented.
- [ ] ORT 1.26.0 path resolution is documented.
- [ ] Windows x64 is explicitly marked as the first supported target.
- [ ] Runtime base directory detection is documented for packaged exe and development project root.

## Pre-Implementation Research

- [ ] Stable Audio 3 model layout and ONNX schema notes are read from cheatsheet.
- [ ] IrodoriTTS model layout and options are read from cheatsheet.
- [ ] Any divergence between research repo and integrated code is recorded.
- [ ] Open questions are resolved or converted into Go / No-Go conditions.

## Traceability

- [ ] Each claim maps to at least one requirement.
- [ ] Each requirement maps to a specification section.
- [ ] Each major specification section has a verification item.
- [ ] Deferred items remain in this plan and are not placed in current docs as implemented facts.
