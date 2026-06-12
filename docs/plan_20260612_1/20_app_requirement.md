# App Requirements

## Purpose

ニュース音声生成待ちを短縮するため、ローカル ONNX 推論を CPU / CUDA / auto から選べるようにする。

## Target Environment

| Target | OS | GPU | Runtime |
| --- | --- | --- | --- |
| Primary | Windows x64 | NVIDIA CUDA | ONNX Runtime 1.26.0 GPU CUDA 13 build + CUDA 13.2 + cuDNN 9.x |
| Fallback | Windows x64 | none / unavailable | ONNX Runtime 1.26.0 CPU build |

## User Experience

- 設定画面で Local Inference の実行プロバイダーを選べる。
- 既定は `auto` とし、CUDA が使えれば GPU、使えなければ CPU で継続する。
- `cuda` を明示した場合、CUDA が使えなければ分かりやすいエラーを表示する。
- `auto` を選んだ場合、CUDA が使えれば CUDA、使えなければ CPU で継続する。
- GPU 版 ORT DLL はリポジトリに含めず、スクリプトで取得する。

## MVP Features

- `localInference.executionProvider`: `cpu` / `cuda` / `auto`
- `localInference.deviceId`: 0 以上の GPU device ID
- GPU 版 ORT DLL のデフォルト探索:
  - 設定 `ortLibraryPath`
  - `FM_RADIO_ORT_LIB`
  - `third_party/onnxruntime-gpu/onnxruntime-win-x64-gpu-1.26.0/lib/onnxruntime.dll`
  - 既存 CPU 版 ORT DLL
- CUDA EP 用 `SessionOptions` を ONNX セッション作成へ渡す。
- 設定 UI に GPU 実行モードと device ID を追加する。
- `cmd/local_smoketest` で CPU と CUDA の生成を検証できるようにする。

## Future Candidates

- DirectML EP による AMD / Intel / NVIDIA 汎用 Windows GPU 対応。
- TensorRT EP 対応。
- IrodoriTTS と Stable Audio 3 の個別 EP 設定。
- UI から GPU 可用性診断を実行するボタン。
- 長文ニュース原稿の分割合成・並列合成。

## Initial Non-Goals

- GPU 版 ORT DLL、CUDA、cuDNN DLL を Git にコミットしない。
- Linux / macOS の GPU 対応は今回の対象外。
- CUDA Graph、量子化、モデル構造変更は対象外。
- 既存 Gemini TTS とファイル BGM の互換性は壊さない。
