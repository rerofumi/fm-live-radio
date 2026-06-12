# Claim

## Purpose

`fm-live-radio` のローカル生成処理を NVIDIA CUDA で高速化し、特に IrodoriTTS のニュース音声生成待ち時間を短縮する。

## Background

2026-06-11 のローカル生成エンジン導入で、Stable Audio 3 による BGM 生成と IrodoriTTS v3 によるニュース音声生成が基本動作するようになった。一方でニュース原稿が長い場合、IrodoriTTS の DiT 反復推論が長く、再生中の先読みだけでは待ち時間が発生しやすい。

CUDA 利用の IrodoriTTS は `E:\programming\AI_generative\VibeCoding\tts-research` の `docs/plan_20260612_1/` で調査・動作確認済みである。この成果を `fm-live-radio` の共通 ONNX Runtime ラッパーへ取り込む。

## Problem

- 現在の `internal/generation` は CPU Execution Provider のみを前提に `NewDynamicAdvancedSession(..., nil)` を使っている。
- CUDA EP を使うには CPU 版ではなく GPU 版 ONNX Runtime DLL と CUDA/cuDNN 依存 DLL が必要である。
- 明示的に GPU を選んだ場合に CPU へ黙って落ちると、設定ミスを検知できない。
- Stable Audio 3 と IrodoriTTS は同じ ORT 環境を共有するため、GPU 設定も一箇所で扱う必要がある。

## Target Users / Environment

- Windows x64
- NVIDIA GPU
- ONNX Runtime 1.26.0 GPU CUDA 13 build
- CUDA Toolkit 13.2 + cuDNN 9.x
- Go/Wails アプリの通常利用者、およびローカル生成の待ち時間を短縮したい開発者

## Initial Scope

- `localInference` 設定に GPU 実行モードと device ID を追加する。
- `internal/generation` に CPU / CUDA / auto の Execution Provider 設定を追加する。
- ORT セッション作成時に CUDA `SessionOptions` を渡せるようにする。
- GPU 版 ORT DLL の取得スクリプトを追加する。
- 設定 UI から CPU / CUDA / auto を選択できるようにする。
- スモークテストで CPU と CUDA の TTS 生成を確認する。

## Initial Technical Hypotheses

- `github.com/yalue/onnxruntime_go` v1.31.0 の `NewSessionOptions`, `NewCUDAProviderOptions`, `AppendExecutionProviderCUDA` を利用できる。
- `internal/localtts/irodori/pipeline` と `internal/musicgen/stableaudio/pipeline` は共通の `generation.NewSession` を呼んでいるため、セッションオプションの追加は共通化できる。
- `auto` は CUDA EP 追加を試し、失敗時は CPU に戻す設計が使いやすい。
- `cuda` 明示指定時は失敗を返し、CPU フォールバックしない。

## Uncertainties

- GPU 版 ORT DLL と CUDA/cuDNN の実ファイル配置が現在の開発環境で揃っているかは実装後に確認する。
- ONNX Runtime 公式 CUDA EP ドキュメントの依存表は CUDA 12 系の説明が中心だが、1.26.0 リリースノートでは CUDA 13 配布物 `gpu_cuda13` が明示されているため、今回はその Windows x64 配布物を採用する。
- Wails 設定保存後、既に ORT 初期化済みのプロセスで EP を変更できない可能性がある。初期化済み設定と異なる場合は再起動を促すエラーにする。
- Stable Audio 3 の CUDA 実行は主目的ではないが、共通 ORT ラッパー経由なので同じ EP 設定が適用される。

## Next Documents

1. `20_app_requirement.md`
2. `docs/cheatsheet/onnxruntime-gpu.md`
3. `30_requirement.md`
4. `40_specification.md`
5. `50_review_notes.md`
6. レビュー承認後に実装
