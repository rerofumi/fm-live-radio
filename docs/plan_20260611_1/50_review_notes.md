# Review Notes

## Review Findings

- `docs/` currently contains only `docs/old/`; current implemented behavior is mostly in README and code. Implementation completion must create or update current docs rather than copying this plan wholesale.
- `go.mod` currently declares `go 1.23`, while both local generation research repos target Go 1.25+ and ONNX Runtime 1.26.0. A Go version bump is likely required.
- Stable Audio 3 and IrodoriTTS research repos both use `internal` packages, so direct module import is not straightforward. Integration needs package copying, refactoring, or extraction into importable modules.
- Stable Audio 3 generation may be slower than a BGM slot duration on CPU. The MVP must include prefetch and `generate_music` cache fallback rather than assuming real-time generation.
- IrodoriTTS は v3 使用で確定。声質 WAV がない場合はデフォルト話者にフォールバックする。
- `Stuble Audio 3` は typo と確認済み。ユーザー向け表記は `Stable Audio 3` に統一し、ローカル調査リポジトリのパスだけ `stuble-audio-3-research` のまま正確に扱う。
- モデル、声質 WAV、生成音楽の配置は `<base>/model`、`<base>/narrator`、`<base>/generate_music` で確定。`<base>` は exe カレントディレクトリまたは開発時プロジェクトルート。
- `narrator` に複数 WAV がある場合は、ファイル一覧取得時の 1 番目を使う。
- `generate_music` フォールバックは、古い順の `n/2` 番目付近を選ぶ。最古ファイルは削除対象になりやすいため避ける。
- 2026-06-11 の追加調査で、ニュース音声が約 1 秒になる原因は RSS 本文欠落ではなく、OpenAI 互換 LLM 呼び出しの `max_tokens: 400` が `gemma4:12b` の thinking に消費され、`choices[0].message.content` が空かつ `finish_reason=length` になることと判明した。`max_tokens: 2000` または未指定では本文が返り、Ollama 上の `gemma4:12b` は model context length 262144、現在ロード中 context 32768 と確認済み。
- 2026-06-12 の追加判断として、IrodoriTTS は 30 秒級の長文一括生成に耐えにくいため、Irodori provider 内でセンテンス分割生成と WAV 結合を行う。Gemini TTS は一括生成の方が品質が良いため分割しない。Irodori の個別センテンス失敗は MVP では 3 秒無音で埋める。

## Fixed Items

- Shared ORT initializer was added under `internal/generation`.
- Stable Audio 3 and IrodoriTTS research packages were copied into the repository and wired behind local services.
- Current docs were added as `docs/requirement.md` and `docs/specification.md`.
- RSS feeds used in the app can provide only short summaries, so `internal/rss` now falls back to article-page extraction for the configured Impress Watch feeds.

## Deferred Items

- Model download UI.
- DirectML/CUDA/TensorRT execution providers.
- Generated music library retention beyond `generate_music` の約 20 件キャッシュ。
- Local LLM bundling.
- Full current-doc rewrite after implementation.

## Rejected Options

- **Delete file BGM and Gemini TTS immediately**: rejected because keeping them as compatibility providers reduces integration risk and preserves current behavior.
- **Run SA3 and IrodoriTTS with separate ORT globals**: rejected because ORT is process-global in the current research implementations and double initialization risks instability.
- **Treat generated BGM as a normal scanned folder**: rejected for MVP because generation has prompt, seed, cancellation, and readiness state that should not be hidden behind filesystem scanning.
- **Fallback to old file BGM when Stable Audio 3 is late**: rejected for the local-generation path. Late generation should fall back to `generate_music` cache.

## Current Non-Goals

- Stable Audio 3 Medium / Large support.
- Model file distribution policy.
- Perfect gapless playback.
- Commercial licensing review.

## Future Plans

- Add bounded generated-asset cache with reuse strategy.
- Add prompt presets by program style.
- Add generated jingle/sweeper slots.
- Add voice preset management for IrodoriTTS.

## Next Plan Candidates

- Model asset setup and installer plan.
- Performance profiling and queue tuning plan.
- Generated audio cache and retention plan.
- Full offline mode with local LLM plan.

## Documentation Feedback

- Keep current docs updated if local generation options or defaults change again.
- Move additional smoke-test findings into `docs/cheatsheet/` after model-backed runtime verification.
- Keep rejected options and unresolved risks in this plan directory.
