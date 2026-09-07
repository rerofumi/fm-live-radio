# レビュー・判断記録

## 決定

2026-09-07: ユーザーが Full / E2E計画 / worklogあり、および Go/ONNX維持・v4.1第一候補を選択した。公式Pythonは調査用の正解系として使用する。Go用の採用可否はWP-1のONNX検証に従う。

## 実行で分かったこと

- v3 Go/CUDA、v4公式CUDA、v4.1公式CUDAのWAV生成が成功した。
- Go tokenizerは6ケースすべて不一致。PADと先頭空白処理の差がある。
- exporterのTextEncoderModuleは実v4.1モデルでAttributeError。export specsもspeaker/durationを欠いた。
- Python 3.12 + sentencepiece 0.1.99のソースビルドはCMake互換性で失敗。Python3.11で同期成功。Windowsの絵文字ログにはUTF-8指定が必要。

## 未解決事項と判断への影響

| 未解決 | 影響 | 解消条件 |
| --- | --- | --- |
| ModernBERT/dual conditioning全体のONNX変換とORT parity | 実装経路が成立するか未確定 | WP-1で全graphと短文生成成功 |
| 多様な原稿・移行後Go/ONNXでの声質・読み | 4サンプルは期待通りとユーザー確認済み。v3比の品質改善と本番経路の品質は未確定 | REQ-09の10原稿×3seed聴取 |
| Go/ONNX性能、CPU実行、BGM併用VRAM | 公式Pythonの速さを移行性能に使えない | REQ-07/08/10の実測 |
| 取消時のsession寿命 | 静的にraceの可能性。実際の不具合再現は未実施 | 取消統合試験と資源所有修正 |
| 公式透かしとGo出力の差 | 波形完全一致の比較条件に影響 | 透かし前tensorをparityの比較点に固定 |

## 採用しない案・延期

- v4初版の既定化: v4.1が公式推奨の後続版であり、両版を実行してv4.1を第一候補とした。
- 公式Python常駐/APIサーバー化: ユーザーがGo/ONNX維持を選択したため今回未採用。ONNXの実現性が成立しない場合のみ再検討する。サーバーAPIは今回実行検証していない。
- 重みだけの差し替え: tokenizerとgraphの非互換を実行で確認したため不可。
- caption UI、長い複数参照、量子化、LoRA: 初回移行の範囲外。公式の説明をもってローカル検証済みとはしない。
- 過去の `plan_202606*_N` ディレクトリ整理: 今回のモデル移行と無関係な既存リンク変更を避け、改名は実施していない。plan_indexに内容別の入口を設ける。

## 文書フィードバック

現行ドキュメントの `cmd/local_smoketest` は現在のツリーに存在しないため、新規検証taskに置換する予定。pipelineの冒頭コメントも実装済みspeaker/durationを未実装と記述しており、v4実装時に修正する。今回は現行v3の仕様を移行済みとして書き換えない。


## ユーザー試聴フィードバック（2026-09-07）

試聴用4ファイルすべてが期待通りの音声品質との報告を受領。[E4記録](evidence/user-audition.md)として保存。v4.1第一候補の方針を維持する。今回の確認を移行後Go/ONNXや全原稿の受入へ拡張しない。
