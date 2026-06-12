# Implementation Specification

## Technical Stack

- Go 1.26 via `mise`
- Wails v2
- `github.com/yalue/onnxruntime_go` v1.31.0
- ONNX Runtime 1.26.0 CPU / GPU CUDA 13 build
- Windows x64 + CUDA 13.2 + cuDNN 9.x for CUDA mode

## File Structure

Modify:

- `internal/domain/types.go`
  - Add `ExecutionProvider string` and `DeviceID int` to `LocalInferenceConfig`.
- `internal/store/store.go`
  - Default `ExecutionProvider` to `auto`.
  - Default `DeviceID` to `0`.
  - Normalize invalid provider values to `cpu`.
- `internal/generation/ort_runtime.go`
  - Add EP config state.
  - Add `ConfigureExecutionProvider(provider string, deviceID int)`.
  - Add `NewSessionOptions` helper for CUDA / auto / CPU.
  - Pass session options to `NewDynamicAdvancedSession`.
  - Replace the default GPU ORT DLL candidate path with `onnxruntime-win-x64-gpu-1.26.0/lib/onnxruntime.dll`, while downloading the `gpu_cuda13` asset.
- `internal/localtts/service.go`
  - Configure generation EP from `cfg.LocalInference` before loading Irodori sessions.
- `internal/musicgen/service.go`
  - Configure generation EP from `cfg.LocalInference` before loading Stable Audio 3 sessions.
- `cmd/local_smoketest/main.go`
  - Accept optional env vars or flags for EP testing:
    - `FM_RADIO_ORT_EP`
    - `FM_RADIO_ORT_DEVICE_ID`
- `frontend/src/App.tsx`
  - Extend `AppConfig.localInference`.
  - Add Settings controls for provider and device ID.
- `frontend/wailsjs/go/models.ts`
  - Regenerate via Wails after Go model changes.
- `docs/cheatsheet/index.md`, `docs/cheatsheet/link.md`
  - Register GPU ORT cheatsheet and local source links.

Add:

- `docs/cheatsheet/onnxruntime-gpu.md`
- `scripts/download_gpu_ort.ps1`
  - Download the CUDA 13 asset and cleanly replace the previous CUDA 12 package directory if it exists.

## Data Model

```go
type LocalInferenceConfig struct {
    ORTLibraryPath    string `json:"ortLibraryPath"`
    MaxWorkers        int    `json:"maxWorkers"`
    ExecutionProvider string `json:"executionProvider"` // cpu, cuda, auto
    DeviceID          int    `json:"deviceId"`
}
```

Internal normalized config:

```go
type EPConfig struct {
    Provider string
    DeviceID int
}
```

Provider values:

- `cpu`: create sessions without explicit EP options.
- `cuda`: append CUDA EP; return error on failure.
- `auto`: default mode. Try CUDA EP first; if it fails, create CPU session and record warning.

## UI / Screens

Settings modal additions:

- `Local Inference Provider`: select `CPU`, `CUDA`, `Auto`
- `Local Inference Device ID`: number input, min `0`

Existing ORT DLL Path remains available. Users can point it directly at GPU build `onnxruntime.dll`.

## Inputs

- Config JSON fields:
  - `localInference.executionProvider`
  - `localInference.deviceId`
- Env vars:
  - `FM_RADIO_ORT_LIB`
  - `IRODORI_ORT_LIB`
  - `SA3_ORT_LIB`
  - `FM_RADIO_ORT_EP`
  - `FM_RADIO_ORT_DEVICE_ID`

Env EP overrides are useful for smoke tests and local debugging. Persisted app config remains the normal UI path.

## Persistence

- Config is stored in the existing `config.json`.
- Missing fields in older configs are defaulted on load and save.

## Error Handling

- Missing provider values default to `auto`; invalid provider values normalize to `cpu`.
- Negative device ID normalizes to `0`.
- Explicit `cuda` failure returns an error like:
  - `cuda execution provider initialization failed: ... Check GPU ONNX Runtime DLL, CUDA 13.2, cuDNN 9.x, and PATH.`
- Changing DLL / EP / device ID after ORT initialization returns an error requiring app restart.
- `auto` failure records a warning and falls back to CPU.

## Import / Export

No new import/export format. Existing settings JSON gains optional fields.

## Environment Constraints

- GPU DLL directory must include:
  - `onnxruntime.dll`
  - `onnxruntime_providers_cuda.dll`
  - `onnxruntime_providers_shared.dll`
- Windows must be able to find CUDA/cuDNN dependent DLLs. Either add CUDA/cuDNN bin dirs to `%PATH%`, or place the required cuDNN DLLs next to ORT GPU DLLs.

## Verification

1. Download GPU ORT:
   - `powershell -ExecutionPolicy Bypass -File scripts/download_gpu_ort.ps1`
2. Build/test:
   - `mise x -- go test ./...`
3. CPU smoke:
   - `mise x -- go run ./cmd/local_smoketest`
4. CUDA smoke:
   - `$env:FM_RADIO_ORT_EP='cuda'; $env:FM_RADIO_ORT_LIB='<gpu-ort-lib>\onnxruntime.dll'; mise x -- go run ./cmd/local_smoketest`
5. Auto smoke:
   - `$env:FM_RADIO_ORT_EP='auto'; mise x -- go run ./cmd/local_smoketest`
6. App manual check:
   - Settings で `IrodoriTTS` と `CUDA` を選択し、ニューストーク生成が非無音 WAV になり、生成時間が CPU より短いことを確認する。
   - 使用 DLL は `third_party/onnxruntime-gpu/onnxruntime-win-x64-gpu-1.26.0/lib/onnxruntime.dll` を基準とする。
