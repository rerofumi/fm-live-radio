# Current Specification

## Backend

- `internal/generation` provides the shared ORT initializer and a small worker queue utility.
- `internal/musicgen` wraps Stable Audio 3 prompt building, generation, cache trimming, and cache fallback selection.
- `internal/localtts` wraps IrodoriTTS synthesis and narrator reference WAV resolution.
- `internal/talk` chooses the TTS provider from config while preserving the RSS and LLM pipeline.
- `internal/player` handles provider-aware BGM selection, Talk prefetch, Music prefetch, and status reporting.

## Defaults

- `<base>` is the current working directory at app startup.
- Stable Audio 3 default model dir: `<base>/model/sa3-sm-music`
- Stable Audio 3 output dir: `<base>/generate_music`
- Irodori default model dir: `<base>/model/irodori-v3`
- Irodori narrator dir: `<base>/narrator`

## Verification Baseline

- `mise x -- go test ./...`
- `mise x -- npm --prefix frontend run build`
- `mise run build`
