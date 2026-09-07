# Cheatsheet Index

| File | Topic | Target | Last Confirmed | Knowledge Type | When To Read |
| --- | --- | --- | --- | --- | --- |
| `local-generation-research.md` | Stable Audio 3 and IrodoriTTS Go local inference integration notes | `stuble-audio-3-research` and `tts-research` local repositories, ONNX Runtime 1.26.0 | 2026-06-11 | Locally verified research summary from local repos | Before implementing local BGM generation, local TTS, ORT setup, or model validation |
| `onnxruntime-gpu.md` | ONNX Runtime CUDA Execution Provider integration notes | ONNX Runtime 1.26.x, `onnxruntime_go` v1.31.0, Windows x64 CUDA | 2026-06-12 | Web-derived via local research + locally verified `tts-research` findings | Before implementing or debugging CUDA / GPU local inference |
| `frontend-visualizer.md` | 常時オンエアの波形ビジュアライザ実装、Web Audio/CORS 制約、RMS envelope 連動案 | `frontend/src/Visualizer.tsx`, Canvas 2D, Wails WebView2, Web Audio API | 2026-06-13 | In-repo implementation knowledge + Web-derived official docs | フロントエンドのビジュアライザ実装・WebAudio 解析・音圧連動の検討時 |
| `irodori-v4-migration.md` | v4/v4.1公式推論、Go tokenizer、ONNX移行条件と実測 | Irodori v4.1 / upstream 8224daf / exporter 5df35d8 / Windows RTX5090 | 2026-09-07 | 公式一次資料 + ローカル実行検証 | v4.1のexporter/tokenizer/推論移行前 |
| `tts-benchmark-e2e.md` | v4.1明示試験の固定原稿ベンチ、試聴manifest、local fixture E2E | `cmd/tts-benchmark`, `cmd/tts-e2e`, CPU/CUDA/auto別プロセス | 2026-09-08 | リポジトリ実装 + ローカル実行検証 | WP-5の性能・E2E・人間試聴 |
| `domain_primer.md` | ローカル音声合成の入力・品質・速度・計算graphの基本 | ローカルTTS/ラジオ | 2026-09-07 | ドメイン説明 + 調査で確認した不変条件 | TTS移行要件やE2Eを作る前 |
