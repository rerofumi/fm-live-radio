# Irodori v4.1 移行計画レビュー

対象版: 2026-09-07 ユーザー試聴反映版 / アプリ基準 db03bec8。

## 利用者にとっての変更

ラジオのニュース読み上げをv3からv4.1へ更新する計画。現在のGo/ONNXローカル実行とnarrator WAVを維持する。Pythonサーバーの起動操作は増やさない。**今回の作業は計画作成までで、アプリの既定モデルはまだv3。**

Go/ONNX維持・v4.1第一候補はユーザー確認済み。追加の方針決定は不要。対応ONNXの実現性と品質の合格を条件として採用する。

## 調査で確定したこと

| 項目 | 結果 |
| --- | --- |
| v3 baseline | Go/CUDAで4.8秒WAVを生成、合成3.028秒（別途ロード6.166秒） |
| v4初版 | 公式CUDAで4.84秒WAVを生成 |
| v4.1 | 公式CUDAで5ケース成功。4.72–5.88秒WAVを合成3.836–4.065秒で生成 |
| 現行tokenizer | 6文すべて公式と不一致。PAD、先頭空白などを修正する必要あり |
| 現行exporter | 実v4.1モデルでtext_encoder構築に失敗。speaker/durationの出力定義も不足 |

公式Pythonと現行Goでは処理条件が違うため、この短文スモークから性能優劣は判断しない。7 WAVは全てPCM16/48kHz/mono/finite/非無音。試聴用4ファイルはユーザーが「期待通りの音声品質」と確認済み。10原稿×3seedと移行後Go/ONNX出力の品質評価は未実施。

## 実施順序

1. **WP-1: ONNX実現性** — 固定公式ソースでv4.1の全graphを変換し、PyTorch/ORT parityと短文生成を確認。ここがGo/No-Goゲート。
2. **WP-2: tokenizer** — 公式ids/maskと完全一致し、v3も壊さない。
3. **WP-3: Go推論** — ModernBERT、speaker patch=4、caption/speaker独立、dual duration、CFGを対応。
4. **WP-4: アプリ統合** — 設定保持、事前検査、取消時の資源管理、複数文生成を整える。
5. **WP-5: 受入** — 10原稿×3seedの品質比較、性能/BGM併用、CPU/CUDA、Wails E2Eを実施。その後のみ新規設定の既定値を更新。

WP-1/2のfixture準備は一部並行可能。Go推論の本実装はgraph契約確定後。失敗時はv3を継続し、既存モデルと設定を使って復帰する。

## 未確認事項と影響

- 対応ONNXは未生成。公式Pythonの成功だけではGo配布版に移行できない。
- narratorは10秒。今回の4サンプルの品質はユーザー確認済み。多様な原稿での声の維持・読みの改善と、移行後Go/ONNXの品質はREQ-09で確認する。
- Go/ONNXの性能、CPU動作、BGMとのメモリ共存、取消/E2Eは未検証。既定切替を止める条件に含めた。
- caption UI、複数参照UI、量子化は初回範囲外。品質評価や機能を済ませた扱いにはしない。

## 試聴用ファイル

同じ短文・同じ既存narratorの生成結果。2026-09-07、ユーザーが以下4ファイルすべてを試聴し、期待通りの音声品質と確認した。[試聴記録と対象hash](evidence/user-audition.md)。

- [現行v3 WAV](../../third_party/irodori-v4-research/v3-baseline.wav)
- [v4初版 WAV](../../third_party/irodori-v4-research/v4-reference.wav)
- [v4.1 WAV](../../third_party/irodori-v4-research/v4.1-reference.wav)
- [v4.1 参照＋caption WAV](../../third_party/irodori-v4-research/v4.1-caption_reference.wav)

音声・重み・venvはgitignore済み調査ディレクトリにあり、リポジトリへは計画と再現手順・小さな証拠だけを残す。

## 詳細と確認項目

[調査と実測](../cheatsheet/irodori-v4-migration.md) → [要件・数値基準](30_requirement.md) → [仕様・E2E・復帰手順](40_specification.md) → [進捗](90_status.md)。[再現手順](evidence/README.md) に実行コマンドを保存。

- [x] v3/v4/v4.1のモデル種別と実行backendを区別した。
- [x] 現行tokenizer/exporterの非互換を実行証拠で示した。
- [x] 既存設定の保持とv3への復帰を定義した。
- [x] 公式Pythonの動作成功とGo/ONNX採用を分けた。
- [ ] 全REQを実装し、別セッション/人が受入する（将来作業）。




