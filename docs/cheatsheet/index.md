# Cheatsheet Index

| File | Topic | Target | Last Confirmed | Knowledge Type | When To Read |
| --- | --- | --- | --- | --- | --- |
| `local-generation-research.md` | Stable Audio 3 and IrodoriTTS Go local inference integration notes | `stuble-audio-3-research` and `tts-research` local repositories, ONNX Runtime 1.26.0 | 2026-06-11 | Locally verified research summary from local repos | Before implementing local BGM generation, local TTS, ORT setup, or model validation |
| `onnxruntime-gpu.md` | ONNX Runtime CUDA Execution Provider integration notes | ONNX Runtime 1.26.x, `onnxruntime_go` v1.31.0, Windows x64 CUDA | 2026-06-12 | Web-derived via local research + locally verified `tts-research` findings | Before implementing or debugging CUDA / GPU local inference |
| `frontend-visualizer.md` | 常時オンエアの波形ビジュアライザ実装、Web Audio/CORS 制約、RMS envelope 連動案 | `frontend/src/Visualizer.tsx`, Canvas 2D, Wails WebView2, Web Audio API | 2026-06-13 | In-repo implementation knowledge + Web-derived official docs | フロントエンドのビジュアライザ実装・WebAudio 解析・音圧連動の検討時 |
