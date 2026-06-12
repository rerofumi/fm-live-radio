# Review Notes

## Review Findings

- `tts-research` 側の調査では、CPU 版 ONNX Runtime DLL では CUDA EP を初期化できないことが確認済み。
- GPU 版 ORT はサイズが大きいため Git へ含めず、取得スクリプトと `.gitignore` 管理にする。
- `fm-live-radio` では Stable Audio 3 と IrodoriTTS が `internal/generation` を共有しているため、EP 設定を各パイプラインへ個別実装せず共通化するのが最も安全。
- ORT はプロセスグローバル初期化なので、起動後に DLL / EP / device ID を変更した場合は再起動が必要になる可能性が高い。
- 現在のユーザー環境は CUDA 13.2 で、既存の CUDA 12 向け ORT パッケージでは実行できない。Windows x64 の `gpu_cuda13` パッケージへ揃える必要がある。

## Fixed Items

- 未実装。ユーザー承認後に実装する。

## Deferred Items

- DirectML / TensorRT / MIGraphX 対応。
- UI 上の GPU 診断ボタン。
- 長文 TTS の分割生成。
- Stable Audio 3 と IrodoriTTS の個別 EP 指定。

## Rejected Options

- GPU DLL をリポジトリへコミットする案: サイズが大きく、クローンと履歴を重くするため採用しない。
- 明示 `cuda` で CPU に黙って落ちる案: 設定ミスを隠すため採用しない。
- IrodoriTTS のみに CUDA EP を直書きする案: Stable Audio 3 と ORT 初期化が重複し、後続メンテナンスが難しくなるため採用しない。

## Current Non-Goals

- 非 Windows GPU 対応。
- AMD / Intel GPU 対応。
- モデル精度変更や量子化。

## Future Plans

- DirectML による Windows 汎用 GPU 対応。
- TTS 原稿分割と生成キュー最適化。
- 生成時間と RTF のステータス表示。

## Next Plan Candidates

- 長文ニュース音声の分割・先読み強化。
- GPU 診断 UI と環境セットアップ確認。

## Documentation Feedback

- 実装後、`docs/requirement.md` と `docs/specification.md` に CUDA 対応済みの現在仕様を反映する。
- GPU 実測値が得られたら `docs/cheatsheet/onnxruntime-gpu.md` に `fm-live-radio` 固有の確認結果を追記する。
