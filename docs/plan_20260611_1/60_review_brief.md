# Review Brief

## Review Purpose

Review the plan to convert `fm-live-radio` into a local-generation AI radio using Stable Audio 3 for BGM and IrodoriTTS for Talk audio, before any code changes are made.

## Review Scope

In scope:

- Backend architecture for local music generation and local TTS.
- Config migration and UI settings scope.
- ONNX Runtime shared initialization.
- Generation queue, prefetch, fallback, and verification strategy.

Out of scope:

- Actual implementation.
- Model download/distribution.
- Stable Audio 3 Medium/Large.
- Full offline LLM replacement.

## Decisions Needed

1. Proceed with Stable Audio 3 small-music as the first BGM generator.
2. Proceed with IrodoriTTS v3 as the first local Talk TTS provider.
3. Keep file BGM and Gemini TTS as fallback providers during migration.
4. Use a shared ORT runtime inside `fm-live-radio` instead of provider-specific ORT globals.
5. Start with one local generation worker to avoid CPU/RAM contention.
6. Accept Windows x64 as the first supported local inference target.
7. Use `<base>/model`, `<base>/narrator`, and `<base>/generate_music` as the default local asset directories.
8. Keep about 20 generated music files and use that cache as the fallback when Stable Audio 3 generation is late.

## Maximum Risks

- CPU generation may not finish before the next playback slot. The approved fallback is `generate_music` cache playback.
- Integrating two `internal`-package research repos may require non-trivial refactoring.
- Go version and CGO requirements may force toolchain changes in `mise.toml` and `go.mod`.
- Large model files and ORT DLL paths can make setup fragile.
- Wails app shutdown and cancellation must avoid leaking ORT sessions or leaving orphaned generation jobs.

## Pre-Implementation Research Status

Local research has been inspected from:

- `E:\programming\AI_generative\VibeCoding\stuble-audio-3-research`
- `E:\programming\AI_generative\VibeCoding\tts-research`

Stable Audio 3 research reports state that ORT 1.26 CPU smoke tests succeeded for T5Gemma, DiT, and SAME-S decoder. IrodoriTTS research reports state that E2E text-to-WAV generation succeeded. These are local research facts and must be re-smoked in `fm-live-radio` after integration.

## Traceability Summary

| Claim | Requirement | Specification | Test / Review |
| --- | --- | --- | --- |
| Avoid finite BGM loop | FR-2 | Music Generation Service | SA3 short WAV smoke, app BGM cycle |
| Avoid Gemini TTS cost | FR-3 | TTS Provider | Irodori v3 short WAV smoke, app Talk cycle |
| Keep radio flow | FR-5 | Player Integration, Generation Queue | `BGM -> Talk -> BGM` manual test |
| Avoid app crash on model failure | FR-4, FR-6 | Error Handling | missing model / missing ORT tests |
| Preserve current behavior | Compatibility, AC-1 | Provider defaults | files + gemini manual test |

## Open Questions

- Should `<base>` prefer exe current directory first and project root only when development markers are found?

## Go / No-Go

Go if:

- User approves this plan.
- Windows x64 + CPU ORT is accepted as MVP target.
- Keeping fallback providers during migration is accepted.
- Model assets can be assumed to exist under `<base>/model` by default.
- `generate_music` cache fallback is accepted for late Stable Audio 3 generation.
- `narrator` multiple-WAV selection uses the first file from file listing.
- `generate_music` fallback selection uses the cache item around the old-order midpoint.

No-Go or revise if:

- The app must remove Gemini and file BGM immediately.
- The app must bundle model assets.
- The MVP must support non-Windows platforms.
- Playback must never wait for generation under any circumstance.
