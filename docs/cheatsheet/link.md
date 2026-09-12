# Cheatsheet Links

## Local Sources

| Source | Source Type | Confirmed | Related File | Why It Matters |
| --- | --- | --- | --- | --- |
| `E:\programming\AI_generative\VibeCoding\stuble-audio-3-research\README.md` | Local research README | 2026-06-11 | `local-generation-research.md` | Stable Audio 3 Go pipeline usage, model layout, ORT requirements |
| `E:\programming\AI_generative\VibeCoding\stuble-audio-3-research\docs\plan_20260608_3\REPORT.md` | Local research report | 2026-06-11 | `local-generation-research.md` | Initial Stable Audio 3 ONNX and Go feasibility analysis |
| `E:\programming\AI_generative\VibeCoding\stuble-audio-3-research\docs\plan_20260608_3\REPORT_session4.md` | Local research report | 2026-06-11 | `local-generation-research.md` | Stable Audio 3 ORT CPU smoke test results and exact ONNX IO names |
| `E:\programming\AI_generative\VibeCoding\tts-research\README.md` | Local research README | 2026-06-11 | `local-generation-research.md` | IrodoriTTS Go usage, options, model layout, runtime constraints |
| `E:\programming\AI_generative\VibeCoding\tts-research\docs\plan_20260608_2\REPORT.md` | Local research report | 2026-06-11 | `local-generation-research.md` | IrodoriTTS E2E text-to-WAV verification and implementation notes |
| `E:\programming\AI_generative\VibeCoding\tts-research\docs\plan_20260612_1` | Local research plan and verification notes | 2026-06-12 | `onnxruntime-gpu.md` | CUDA EP design, rejection decisions, and verified GPU TTS behavior |
| `E:\programming\AI_generative\VibeCoding\tts-research\docs\cheatsheet\onnxruntime_gpu.md` | Local CUDA cheatsheet | 2026-06-12 | `onnxruntime-gpu.md` | ORT GPU DLL layout, Go API usage, CUDA/cuDNN failure patterns |

## Web Sources Mentioned By Local Research

These links were referenced by the local research reports but were not revalidated during this planning pass.

| URL | Source Type | Confirmation Date | Related File | Reason |
| --- | --- | --- | --- | --- |
| https://github.com/Stability-AI/stable-audio-3 | Upstream repository | Not revalidated on 2026-06-11 | `local-generation-research.md` | Stable Audio 3 upstream implementation |
| https://huggingface.co/stabilityai/stable-audio-3-optimized/tree/main/onnx | Model hosting | Not revalidated on 2026-06-11 | `local-generation-research.md` | Stable Audio 3 optimized ONNX assets |
| https://github.com/Aratako/Irodori-TTS | Upstream repository | Not revalidated on 2026-06-11 | `local-generation-research.md` | IrodoriTTS upstream implementation |

## Web Sources Revalidated By CUDA Research

These links were revalidated in `tts-research` on 2026-06-12 and copied here as implementation inputs for `onnxruntime-gpu.md`.

| URL | Source Type | Confirmation Date | Related File | Reason |
| --- | --- | --- | --- | --- |
| https://onnxruntime.ai/docs/execution-providers/ | Official docs | 2026-06-12 | `onnxruntime-gpu.md` | Execution Provider status and platform support |
| https://onnxruntime.ai/docs/execution-providers/CUDA-ExecutionProvider.html | Official docs | 2026-06-12 | `onnxruntime-gpu.md` | CUDA EP requirements, options, and compatibility |
| https://github.com/microsoft/onnxruntime/releases | Release notes | 2026-06-12 | `onnxruntime-gpu.md` | GPU ORT distribution source and version |
| https://github.com/yalue/onnxruntime_go | Library repository | 2026-06-12 | `onnxruntime-gpu.md` | Go binding CUDA EP support |
| https://pkg.go.dev/github.com/yalue/onnxruntime_go | API docs | 2026-06-12 | `onnxruntime-gpu.md` | `SessionOptions` and `CUDAProviderOptions` APIs |

## Web Sources Revalidated By Visualizer Loudness Research

| URL | Source Type | Confirmation Date | Related File | Reason |
| --- | --- | --- | --- | --- |
| https://www.w3.org/TR/webaudio-1.1/ | W3C specification | 2026-06-13 | `frontend-visualizer.md` | `MediaElementAudioSourceNode` cross-origin security behavior; CORS-cross-origin resources must output silence |
| https://developer.mozilla.org/en-US/docs/Web/API/AnalyserNode | MDN official docs | 2026-06-13 | `frontend-visualizer.md` | `AnalyserNode` real-time frequency/time-domain analysis behavior |
| https://developer.mozilla.org/en-US/docs/Web/API/AnalyserNode/getByteTimeDomainData | MDN official docs | 2026-06-13 | `frontend-visualizer.md` | Time-domain waveform data can be sampled for RMS without spectrum UI |
| https://developer.mozilla.org/en-US/docs/Web/API/AudioContext/createMediaElementSource | MDN official docs | 2026-06-13 | `frontend-visualizer.md` | `createMediaElementSource()` reroutes media element playback into the AudioContext graph |
| https://developer.mozilla.org/en-US/docs/Web/API/HTMLMediaElement/crossOrigin | MDN official docs | 2026-06-13 | `frontend-visualizer.md` | Required CORS mode behavior for media element resource fetches |
| https://developer.mozilla.org/en-US/docs/Web/API/HTMLMediaElement/captureStream | MDN official docs | 2026-06-13 | `frontend-visualizer.md` | `captureStream()` capability and Limited availability status |
| https://wails.io/docs/guides/windows/ | Wails official docs | 2026-06-13 | `frontend-visualizer.md` | Windows Wails runtime dependency on Microsoft WebView2 |

## Irodori v4 / v4.1 調査（2026-09-07）

関連資料: `irodori-v4-migration.md`。以下は一次資料。実行証拠は同資料から辿る。

| URL | 種類 | 確認日 | 用途 |
| --- | --- | --- | --- |
| https://huggingface.co/Aratako/Irodori-TTS-v4-Small/blob/4c92c7ee2bb15c19a97cf4e86d24fd6bf33b0135/README.md | 公式モデルカード | 2026-09-07 | v4構造・v4.1推奨 |
| https://huggingface.co/Aratako/Irodori-TTS-v4.1-Small/blob/2b28324dc263ed5e6638b3cf3dd94c82ead07b4b/README.md | 公式モデルカード | 2026-09-07 | duration更新・評価範囲・制約 |
| https://github.com/Aratako/Irodori-TTS/tree/8224dafb46d0aba89209a8f905f1cb7e3299d9c1 | 公式推論実装 | 2026-09-07 | 実行したRuntimeKey/SamplingRequest、model config、依存 |
| https://github.com/mtsmfm/Irodori-TTS-ONNX/tree/5df35d8720f810902971745a5ad961ff436bd73c | 既存exporter一次ソース | 2026-09-07 | wrapper/APIと旧fork固定の確認 |
| https://github.com/Aratako/Irodori-TTS-Server | 公式サーバー | 2026-09-07 | 別プロセス案の存在確認のみ。API実行未検証・未採用 |

## 2026-09-12 生成調停のローカル確認

外部資料の再検証・新規 API 採用なし。確認版・実行条件と結果は [generation-scheduling.md](generation-scheduling.md)。

| Source | Source Type | Confirmed | Related File | Why It Matters |
| --- | --- | --- | --- | --- |
| [generation/arbiter.go](../../internal/generation/arbiter.go)、[localtts/service.go](../../internal/localtts/service.go)、[musicgen/service.go](../../internal/musicgen/service.go) | 実装後の Go as-built | 2026-09-12 | `generation-scheduling.md` | 調停の適用範囲、Runtime の寿命、join/Close、予約転送 |
| [player.go](../../internal/player/player.go)、[cache.go](../../internal/musicgen/cache.go)、[audio/server.go](../../internal/audio/server.go)、[fileprotect/guard.go](../../internal/fileprotect/guard.go) | 実装後の Go as-built | 2026-09-12 | `generation-scheduling.md` | 2曲先読み、genre epoch、fallback provenance、WAV/token 参照保護 |
| [mise.toml](../../mise.toml)、[frontend/package.json](../../frontend/package.json) | 現行コマンド定義と実行確認 | 2026-09-12 | `generation-scheduling.md` | Go/race/frontend/Wails build の検証条件 |
