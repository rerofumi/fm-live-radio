# Review Brief

## Review Purpose

`fm-live-radio` のローカル ONNX 推論へ CUDA Execution Provider を追加する計画を、コード編集前にレビューする。

## Review Scope

- `internal/generation` の共通 ORT 初期化・セッション作成。
- `localInference` 設定モデルと Settings UI。
- GPU 版 ONNX Runtime DLL の取得・配置。
- CPU 互換性、CUDA 明示失敗、auto フォールバック。
- IrodoriTTS のニュース音声生成高速化。

## Decisions Needed

1. `localInference.executionProvider` の値を `cpu` / `cuda` / `auto` にする。
2. 既定値は `auto` にし、CUDA 不可時は CPU にフォールバックする。
3. `cuda` 明示時は失敗を返し、`auto` のみ CPU フォールバックする。
4. GPU DLL は Git に含めず `scripts/download_gpu_ort.ps1` で取得する。
5. GPU 設定は Stable Audio 3 と IrodoriTTS の共通 ORT 層へ適用する。

## Maximum Risks

- ORT はプロセスグローバル初期化なので、起動後の EP 変更が反映できない。
- CUDA/cuDNN DLL 探索に失敗すると Windows load error になりやすい。
- `auto` フォールバックの警告が UI で分かりにくい可能性がある。
- CUDA EP は共通層へ入るため、Stable Audio 3 にも影響する。

## Pre-Implementation Research Status

- `tts-research` で CUDA EP の調査・動作確認済み。
- `onnxruntime_go` v1.31.0 の `SessionOptions` / `CUDAProviderOptions` API はローカルモジュールでも確認済み。
- ORT 1.26.0 の Windows x64 GPU 配布物は `gpu_cuda13` を利用し、CUDA 13.2 + cuDNN 9.x 前提で扱う。
- GPU DLL は `third_party/onnxruntime-gpu/` へ展開し、Git 追跡しない。

## Traceability Summary

| Claim | Requirement | Specification | Test / Review |
| --- | --- | --- | --- |
| TTS 待ち時間短縮 | FR-2 CUDA EP | `internal/generation` SessionOptions | CUDA `local_smoketest` |
| CPU 互換維持 | FR-1, FR-4 | default `auto`, CPU DLL fallback | CPU `local_smoketest`, `go test` |
| 設定ミスを隠さない | FR-2, FR-3 | `cuda` error / `auto` fallback | CUDA DLL 不在テスト |
| GPU DLL をコミットしない | FR-6 | download script + `third_party/` ignore | VCS status review |
| UI から選択可能 | FR-5 | Settings select + device ID | Wails app manual check |

## Open Questions

- 現在の実行環境に GPU ORT DLL と cuDNN 9 DLL が揃っているか。
- `auto` フォールバック警告を既存 `localGenerationError` に出すだけで十分か。

## Go / No-Go

- Go: この設計で実装へ進める。CPU 互換を維持し、CUDA は明示失敗、auto はフォールバックという挙動が明確。
- No-Go 条件: ユーザーが「設定 UI は不要」「TTS のみ GPU」など追加のスコープ変更を希望する場合は、`30_requirement.md` と `40_specification.md` を更新してから実装する。
