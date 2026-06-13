# App Requirements

## Purpose

ローカル生成専用の AI ラジオとして、ユーザーが必要最小限のアプリ機能設定と詳細な生成設定を分けて扱えるようにする。

## Target Environment

- Windows x64
- Wails v2 + Go + React
- Stable Audio 3 と IrodoriTTS v3 のローカルモデル
- ONNX Runtime CPU / CUDA

## User Experience

- 通常利用で触る項目は Settings の「アプリ機能」にまとまっている。
- モデル path、ONNX Runtime、生成 steps などの詳細項目は「生成設定」にまとまり、デフォルトでは閉じている。
- 旧 provider の選択肢は表示されない。
- 再生操作画面にはファイル BGM 用の Genre select を表示しない。
- Play すると Stable Audio 3 BGM と IrodoriTTS Talk の生成・再生フローだけが動く。

## MVP Features

- BGM は Stable Audio 3 固定。
- Talk TTS は IrodoriTTS v3 固定。
- RSS、LLM、Talk 周期、音量、無音 gap は「アプリ機能」で編集できる。
- ORT、Stable Audio 3、IrodoriTTS の model / generation parameters は「生成設定」で編集できる。
- 旧 config field は新規保存時に出力されない。

## Future Candidates

- 生成設定のプリセット保存。
- Irodori 話者プリセット管理。
- Stable Audio 3 prompt preset 管理。

## Initial Non-Goals

- ファイル BGM provider を fallback として残すこと。
- Gemini TTS provider を fallback として残すこと。
- 設定ファイルの migration version を新設すること。
- 既存ユーザーの旧 provider 設定値を別ファイルへ退避すること。
