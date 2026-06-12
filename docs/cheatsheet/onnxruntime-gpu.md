# ONNX Runtime GPU Notes

Confirmed: 2026-06-12 / ONNX Runtime 1.26.x / `github.com/yalue/onnxruntime_go` v1.31.0

## Scope

Reusable notes for enabling GPU Execution Providers in `fm-live-radio` local ONNX inference.

Primary source is the locally verified `tts-research` CUDA work:

- `E:\programming\AI_generative\VibeCoding\tts-research\docs\plan_20260612_1`
- `E:\programming\AI_generative\VibeCoding\tts-research\docs\cheatsheet\onnxruntime_gpu.md`

## CUDA Requirements

| Component | Version / Requirement |
| --- | --- |
| ONNX Runtime | 1.26.0 GPU CUDA 13 build |
| Go binding | `github.com/yalue/onnxruntime_go` v1.31.0 |
| CUDA Toolkit | 13.2 |
| cuDNN | 9.x |
| Platform | Windows x64 for the current MVP |

GPU ORT DLL layout:

```text
onnxruntime-win-x64-gpu-1.26.0/
  lib/
    onnxruntime.dll
    onnxruntime_providers_cuda.dll
    onnxruntime_providers_shared.dll
    onnxruntime_providers_tensorrt.dll
```

The provider DLLs must sit next to `onnxruntime.dll`. Windows must also find CUDA/cuDNN DLLs via `%PATH%` or the same directory.
The CUDA 13 release asset name is `onnxruntime-win-x64-gpu_cuda13-1.26.0.zip`, but its extracted directory name remains `onnxruntime-win-x64-gpu-1.26.0`.

## Go API Pattern

```go
sessionOptions, err := onnxruntime_go.NewSessionOptions()
if err != nil {
    return err
}
defer sessionOptions.Destroy()

cudaOptions, err := onnxruntime_go.NewCUDAProviderOptions()
if err != nil {
    return err
}
defer cudaOptions.Destroy()

if err := cudaOptions.Update(map[string]string{"device_id": "0"}); err != nil {
    return err
}
if err := sessionOptions.AppendExecutionProviderCUDA(cudaOptions); err != nil {
    return err
}

session, err := onnxruntime_go.NewDynamicAdvancedSession(
    modelPath, inputNames, outputNames, sessionOptions,
)
```

`CUDAProviderOptions` can be destroyed after `AppendExecutionProviderCUDA`. `SessionOptions` can be destroyed after session creation.

## Decision Rules

- Default to `auto` for user-facing app settings so CUDA is used when available and CPU remains the fallback.
- Use `cuda` when the user explicitly wants NVIDIA GPU acceleration; fail clearly if initialization fails.
- Use `auto` when the user wants best-effort acceleration; try CUDA first and fall back to CPU with a warning.
- Do not commit GPU ORT binaries; use a download script under `scripts/`.
- For the current workstation, prefer the CUDA 13 package over the older CUDA 12 package because CUDA 13.2 is installed and the target GPU is RTX 5090.

## Common Failure Patterns

- CPU ORT DLL loaded: `AppendExecutionProviderCUDA` fails because CUDA EP is not compiled in.
- Missing `onnxruntime_providers_cuda.dll`: provider append fails or dependent DLL load fails.
- Missing cuDNN 9 DLLs on Windows: typically reports Windows load error 126.
- Wrong cuDNN major: cuDNN 8.x is incompatible with ORT 1.20+ CUDA builds that require cuDNN 9.x.

## Verification Commands

CPU:

```powershell
mise x -- go run ./cmd/local_smoketest
```

CUDA:

```powershell
$env:FM_RADIO_ORT_EP = 'cuda'
$env:FM_RADIO_ORT_LIB = 'E:\programming\AI_generative\fm-live-radio\third_party\onnxruntime-gpu\onnxruntime-win-x64-gpu-1.26.0\lib\onnxruntime.dll'
mise x -- go run ./cmd/local_smoketest
```

Auto:

```powershell
$env:FM_RADIO_ORT_EP = 'auto'
mise x -- go run ./cmd/local_smoketest
```

## Update Conditions

Update this note when:

- ONNX Runtime version changes.
- `onnxruntime_go` version changes.
- CUDA/cuDNN major version requirements change.
- `fm-live-radio` local smoke test results differ from the expected CPU / CUDA behavior.
