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
