# Current Requirements

## Playback Modes

- BGM source supports `files` and `stable_audio_3`.
- TTS source supports `gemini` and `irodori`.
- Existing `files + gemini` behavior remains available as a compatibility path.

## Local Generation

- Stable Audio 3 generates WAV files into `<base>/generate_music` by default.
- IrodoriTTS v3 generates Talk WAV files from the existing RSS -> LLM script flow.
- ORT library path can be supplied by config `localInference.ortLibraryPath` or `FM_RADIO_ORT_LIB`.
- When Stable Audio 3 generation is late or fails, cached `generate_music` WAV files are used when available.
- Cache fallback chooses an old-order midpoint item instead of the oldest file.
- When `narrator` contains multiple WAV files, the first listed file is used as the reference WAV.
- When `narrator` has no WAV files, IrodoriTTS runs without a reference WAV.

## UI

- Settings exposes BGM source, TTS source, ORT DLL path, Stable Audio 3 paths/options, and Irodori paths/options.
- Status exposes Talk prefetch state, Music prefetch state, and local generation error text.
