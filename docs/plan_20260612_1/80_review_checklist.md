# Review Checklist

## Security

- [ ] No API keys, model files, or binary DLLs are committed.
- [ ] Download script uses the official ONNX Runtime release URL.

## Frontend

- [ ] Settings has provider select and device ID input.
- [ ] Existing ORT DLL path input remains usable.
- [ ] Default `auto` keeps existing CPU users working via fallback.

## Backend

- [ ] `internal/generation` remains the only ORT lifecycle owner.
- [ ] CUDA `SessionOptions` are passed to every shared ONNX session.
- [ ] `CUDAProviderOptions` and `SessionOptions` are destroyed after use.
- [ ] Explicit `cuda` failure does not silently fall back.
- [ ] `auto` fallback records a visible warning.
- [ ] ORT reconfiguration after initialization is rejected safely.

## DB / Storage

- [ ] Existing config JSON without new fields loads successfully.
- [ ] Missing provider values default to `auto`, invalid provider values normalize to `cpu`.
- [ ] Negative device ID normalizes to `0`.

## QA / Test

- [ ] `mise x -- go test ./...` passes.
- [ ] CPU `cmd/local_smoketest` passes.
- [ ] CUDA `cmd/local_smoketest` passes when GPU dependencies exist.
- [ ] CUDA failure path is checked with CPU ORT DLL or missing provider DLL.
- [ ] `auto` fallback is checked without CUDA.

## DevOps / Environment

- [ ] `scripts/download_gpu_ort.ps1` downloads ORT 1.26.0 `gpu_cuda13` build.
- [ ] `third_party/` remains ignored.
- [ ] CUDA 13.2 and cuDNN 9.x requirements are documented.
- [ ] Commands are run via `mise`.

## Pre-Implementation Research

- [x] `tts-research` CUDA plan and notes were reviewed.
- [x] `onnxruntime_go` v1.31.0 CUDA API was checked locally.
- [x] ORT GPU DLL layout and cuDNN failure pattern were recorded in cheatsheet.

## Traceability

- [ ] Claim, requirements, specification, and tests cover CPU compatibility.
- [ ] Claim, requirements, specification, and tests cover CUDA acceleration.
- [ ] Deferred DirectML / TensorRT / MIGraphX items remain out of current scope.
