# Requirements

## Scope

`fm-live-radio` の共通 ONNX Runtime 層へ CUDA Execution Provider 対応を追加し、ローカル TTS 生成を GPU で実行できるようにする。

## Functional Requirements

### FR-1: Execution Provider 設定

- アプリ設定 `localInference` に `executionProvider` を追加する。
- 値は `cpu`, `cuda`, `auto` のいずれか。
- 未設定は `auto`、不正値は `cpu` として扱う。
- `localInference.deviceId` を追加し、0 以上の整数として扱う。

### FR-2: CUDA EP セッション作成

- `executionProvider == "cuda"` の場合、`onnxruntime_go.NewSessionOptions` と `AppendExecutionProviderCUDA` を使って CUDA EP を追加する。
- CUDA EP 追加に失敗した場合はエラーを返し、CPU へ黙ってフォールバックしない。
- エラーには GPU 版 ORT DLL、CUDA 13.2、cuDNN 9.x、PATH / DLL 配置確認を含める。

### FR-3: Auto モード

- `executionProvider == "auto"` の場合、CUDA EP 追加を試す。
- 成功した場合は CUDA を使用する。
- 失敗した場合は CPU でセッションを作成し、警告を `localGenerationError` など既存のエラー表示経路で確認できるようにする。

### FR-4: ORT DLL 探索

- 明示された `ortLibraryPath` を最優先する。
- 次に `FM_RADIO_ORT_LIB`, `IRODORI_ORT_LIB`, `SA3_ORT_LIB` を見る。
- 既定探索に GPU 版 ORT DLL パスを CPU 版より前に追加する。
- GPU DLL が存在しない環境でも CPU 版の既存探索は維持する。

### FR-5: 設定 UI

- Settings に Local Inference の実行プロバイダー選択を追加する。
- Device ID の数値入力を追加する。
- 既存 ORT DLL path 入力は維持する。

### FR-6: GPU ORT 取得スクリプト

- `scripts/download_gpu_ort.ps1` を追加し、`onnxruntime-win-x64-gpu_cuda13-1.26.0.zip` を `third_party/onnxruntime-gpu/` へ展開できるようにする。
- `gpu_cuda13` asset の展開先ディレクトリ名は `onnxruntime-win-x64-gpu-1.26.0/` のままであることを前提に、実装と手順を更新する。
- `third_party/` は既に `.gitignore` 対象なので、DLL は追跡しない。

## Non-Functional Requirements

- 既定値は `auto` とし、GPU 未導入環境でも CPU にフォールバックして既存ユーザーを壊さない。
- ORT 環境はプロセス単位で一度だけ初期化する。
- 初期化済みの ORT と異なる DLL / EP / device ID を後から指定した場合は、安全にエラーを返す。
- CUDA オプション、セッションオプションは作成後に適切に破棄する。

## Constraints

- `jj` 管理リポジトリとして扱う。
- コマンドは `mise` 経由を優先する。
- Go / Wails コマンドは直接実行せず `mise x -- ...` を使う。
- コード編集前にこの計画のユーザーレビューと承認を受ける。

## Compatibility

- 既存 CPU 実行の Stable Audio 3 / IrodoriTTS は動作を維持する。
- 既存設定ファイルに新フィールドが無くても `applyConfigDefaults` で補完する。
- Wails の TypeScript モデルは Go 側変更後に再生成する。

## Out of Scope

- DirectML / TensorRT / MIGraphX。
- GPU メモリ使用量の詳細チューニング。
- モデルファイル自体の最適化。
- 長文 TTS の分割生成。

## Acceptance Criteria

- `mise x -- go test ./...` が成功する。
- `mise x -- go run ./cmd/local_smoketest` が CPU で成功する。
- GPU 版 ORT と CUDA/cuDNN がある環境で、`FM_RADIO_ORT_EP=cuda` または設定 `cuda` により IrodoriTTS が非無音 WAV を生成する。
- CUDA が使えない状態で `cuda` を指定すると、原因確認先を含む明確なエラーになる。
- `auto` 指定時は CUDA 不可でも CPU 生成にフォールバックする。
