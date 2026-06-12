# fm-live-radio

![Screenshot_mapmode](docs/screenshot_fm-live-radio_2025-12-13-a.png)

Wails + Go + React で作った AI ローカルラジオです。  
BGM を流しながら RSS から記事を選び、LLM で原稿を作り、TTS でニューストークを差し込みます。

現在は以下の構成で動きます。

- BGM:
  - 手元の音楽ファイル
  - Stable Audio 3 によるローカル生成
- Talk:
  - Gemini TTS
  - IrodoriTTS v3 によるローカル生成
- Local inference:
  - ONNX Runtime CPU
  - ONNX Runtime CUDA (`auto` / `cuda` / `cpu`)

## この README の対象

この README は、初回セットアップから `mise run dev` で起動し、ローカル生成と GPU 利用まで確認するための手順をまとめたものです。

## 必要なもの

### 必須

- Windows x64
- `mise`
- `uv`
- OpenAI 互換 API
  - 例: Ollama, LM Studio, OpenRouter
- RSS URL

### Talk を Gemini で使う場合

- Gemini API Key

### ローカル生成を使う場合

以下のファイルやディレクトリが必要です。

- Stable Audio 3 モデル:
  - `model/sa3-sm-music/`
- IrodoriTTS v3 モデル:
  - `model/irodori-v3/`
- 話者参照 WAV:
  - `narrator/*.wav`
  - 参照 WAV がなくても IrodoriTTS v3 のデフォルト話者では動きます

このリポジトリでは、開発用の既定パスは次です。

- Stable Audio 3:
  - `model/sa3-sm-music`
- IrodoriTTS:
  - `model/irodori-v3`
- narrator:
  - `narrator`

## 最初にやること

### 1. `mise` を入れる

```powershell
winget install jdx.mise
```

### 2. ツールチェーンを入れる

```powershell
mise install
```

### 3. Wails CLI を入れる

```powershell
mise run setup
```

### 4. GPU を使うなら ONNX Runtime と依存 DLL を入れる

このプロジェクトでは GPU 用ファイルを Git に含めていません。  
初回だけ次を実行してください。

```powershell
powershell -ExecutionPolicy Bypass -File scripts\download_gpu_ort.ps1
```

このスクリプトは次を行います。

- ONNX Runtime CUDA 13 asset
  - `onnxruntime-win-x64-gpu_cuda13-1.26.0.zip`
  をダウンロード
- `third_party/onnxruntime-gpu/onnxruntime-win-x64-gpu-1.26.0/` に展開
- `uv` を使って `nvidia-cudnn-cu13==9.23.1.3` を `third_party/` 配下へ展開
- `cuDNN`, `cuBLAS`, `nvrtc` の必要 DLL を ORT の `lib` にコピー

展開後に使われる `onnxruntime.dll` は次です。

- `third_party/onnxruntime-gpu/onnxruntime-win-x64-gpu-1.26.0/lib/onnxruntime.dll`

### 5. スモークテストを回す

CPU / GPU の切り分けに使います。

```powershell
mise x -- go run ./cmd/local_smoketest
```

GPU 強制確認:

```powershell
$env:FM_RADIO_ORT_EP='cuda'
mise x -- go run ./cmd/local_smoketest
```

終わったら環境変数を消してください。

```powershell
Remove-Item Env:FM_RADIO_ORT_EP
```

## 起動

### 開発起動

```powershell
mise run dev
```

### 本番ビルド

```powershell
mise run build
```

## 初回起動後に Settings で設定する項目

最低限、以下を埋めれば動きます。

- `RSS URLs`
- `LLM Base URL`
- `LLM Model`
- `TTS Source`
- `BGM Source`

用途別の推奨設定:

### ローカル Talk + ローカル BGM

- `TTS Source` = `irodori`
- `BGM Source` = `stable_audio_3`
- `Local Inference Provider` = `auto`
- `ORT DLL Path` = 空でよい
  - 既定探索で GPU ORT を拾います

### ローカル Talk + ファイル BGM

- `TTS Source` = `irodori`
- `BGM Source` = `files`
- `BGM Root Path` = 音楽フォルダ

### Gemini Talk + ローカル BGM

- `TTS Source` = `gemini`
- `Gemini API Key` を設定
- `TTS Model` / `TTS Voice` を設定

## GPU 利用の考え方

- `Local Inference Provider` の既定値は `auto`
- `auto`
  - CUDA が使えれば GPU
  - 使えなければ CPU へフォールバック
- `cuda`
  - GPU を強制
  - 失敗時はエラー
- `cpu`
  - 常に CPU

起動ログで次が出れば GPU 版 ORT を読めています。

```text
INFO: using ONNX Runtime shared library: third_party\onnxruntime-gpu\onnxruntime-win-x64-gpu-1.26.0\lib\onnxruntime.dll
```

このログが出ていて、さらに `Local Inference Provider = cuda` で生成が通るなら GPU 実行です。

## このリポジトリで増えた「初回に必要な関連ファイル」

初回セットアップで意識すべきものをまとめると次です。

### リポジトリ内

- `scripts/download_gpu_ort.ps1`
- `cmd/local_smoketest/main.go`
- `model/sa3-sm-music`
- `model/irodori-v3`
- `narrator`

### スクリプト実行後に生成されるもの

- `third_party/onnxruntime-gpu/onnxruntime-win-x64-gpu-1.26.0/`
- `third_party/nvidia-cudnn-cu13/`

これらは Git 管理対象ではなく、初回セットアップで作られるローカル依存物です。

## 設定ファイルの保存先

設定や履歴は `os.UserConfigDir()` 配下に保存されます。  
この環境では次です。

- `C:\Users\rero2\AppData\Roaming\fm-live-radio\config.json`
- `C:\Users\rero2\AppData\Roaming\fm-live-radio\history.json`
- `C:\Users\rero2\AppData\Roaming\fm-live-radio\temp_audio\`

`temp_audio/` は起動時に掃除されます。

## 開発メモ

- `go`, `npm`, `wails` は直接実行しない
- 必ず `mise x -- ...` か `mise run ...` を使う
- Go 側 API を変えたら:

```powershell
mise x -- wails generate module
```

- フロント単体ビルド確認:

```powershell
mise x -- npm --prefix frontend run build
```

- Go 側確認:

```powershell
mise x -- go test ./...
```

## トラブル時の最短確認

### CPU fallback になった

1. `scripts\download_gpu_ort.ps1` を再実行
2. `mise x -- go run ./cmd/local_smoketest`
3. `Local Inference Provider = cuda` で再確認

### ORT の読み先を固定したい

Settings の `ORT DLL Path` に次を入れます。

`E:\programming\AI_generative\fm-live-radio\third_party\onnxruntime-gpu\onnxruntime-win-x64-gpu-1.26.0\lib\onnxruntime.dll`

### 何を読んでいるか確認したい

起動ログの `INFO: using ONNX Runtime shared library: ...` を見ます。

## Security

- API キーは `config.json` に保存されます
- ログ共有時は API キーや個人パスを必要に応じてマスクしてください

## License

MIT
