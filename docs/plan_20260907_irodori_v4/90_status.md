# 移行計画ステータス

state: planned
worklog: enabled
tier: Full
plan: plan_20260907_irodori_v4

2026-09-07: 調査・動作確認・計画文書の作成まで完了。製品の移行実装は未着手。Go/ONNX維持・v4.1第一候補はユーザー確認済み。ONNX採用は条件付き、WP-1を最初に実施する。

## 受入表

| ID | 要件概要 | 検証方法 | 証拠種別 | WP | 実装状態 | 証拠 | 受入 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| REQ-01 | 固定環境とモデル一式 | manifest/lock/export再現 | E1/E3 | WP-1 | todo | 調査のみ: evidence/environment.json。対応export未作成 | 未実施 |
| REQ-02 | tokenizer一致 | 公式ids/mask fixtures | E1 | WP-2 | todo | 現行失敗: evidence/tokenizer-parity.json | 未実施 |
| REQ-03 | v4.1 ONNX graph | CPU/CUDA parityと欠損検査 | E1 | WP-1/3 | todo | 現行失敗: evidence/exporter-compat.json | 未実施 |
| REQ-04 | 独立条件とduration/CFG | 4条件/境界tensor検証 | E1/E2 | WP-3 | todo | 公式Python実測のみ: evidence/v4.1-runtime.json | 未実施 |
| REQ-05 | アプリの音声生成契約 | 実WAV/文分割統合 | E1/E2 | WP-3/4 | todo | 調査WAVのみ: evidence/wav-validation.json | 未実施 |
| REQ-06 | 設定保持/事前検査/復帰 | 設定と異常系試験 | E1/E2 | WP-4 | todo | - | 未実施 |
| REQ-07 | ORT/BGM共存と取消 | EP/取消/連続運転 | E1/E2 | WP-4/5 | todo | - | 未実施 |
| REQ-08 | 性能とメモリ | 10原稿/20反復/期限計測 | E2 | WP-5 | todo | 短文スモークは基準通過証拠ではない | 未実施 |
| REQ-09 | 読みとナレーター品質 | 10原稿×3seed聴取 | E4 | WP-5 | todo | [ユーザー試聴](evidence/user-audition.md): 4件とも期待通り | 4件の事前品質確認済み。10原稿×3seed・移行後出力は未実施 |
| REQ-10 | E2E/ビルド/文書 | core E2Eと回帰 | E1/E2/E3 | WP-5 | todo | 現行周辺go testのみ: evidence/go-check.log | 未実施 |

## 調査証拠（実装・受入とは別）

- 公式v4/v4.1 CUDA、現行v3 CUDA実行: 成功。7件のWAV検証成功。
- tokenizer比較: 6/6不一致。既存exporter: 実モデルwrapper構築失敗を再現。
- 公式Python依存: 3.12失敗→3.11成功。UTF-8出力指定で絵文字ログ成功。
- `go test ./internal/localtts/... ./internal/audiofmt/... ./internal/store/...`: 成功。localttsにはテストなし。
- ユーザーによる試聴用4件の事前品質確認は完了（E4）。実装後の全要件受入は未実施であり、stateはplannedを維持する。

## 次の作業

WP-1でtext/caption encoderの最小ONNX変換から着手し、全graph parityへ進む。REQ-03が成立する前にstoreの既定値をv4.1へ更新しない。


