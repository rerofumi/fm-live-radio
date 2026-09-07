# 移行計画ステータス

state: implementing
worklog: enabled
tier: Full
plan: plan_20260907_irodori_v4

2026-09-07: WP-1完了・独立受入pass。exporter/固定資産と6 ONNX graph、CPU/CUDA・40step生成ループ・隔離環境再現を確認し、Go/ONNX移行の技術Goと判定。Go製品実装と既定値はv3のまま。WP-2以降が残るため計画全体はimplementing。

## 受入表

| ID | 要件概要 | 検証方法 | 証拠種別 | WP | 実装状態 | 証拠 | 受入 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| REQ-01 | 固定環境とモデル一式 | manifest/lock/export再現 | E1/E3 | WP-1 | implemented | [最終manifest](evidence/wp1-manifest.json)、[固定入力検証](evidence/acceptance-fixed-inputs.json)、[再export](evidence/acceptance-reexport-parity.json)、[手順](../../tools/irodori_export/README.md) | pass（[独立受入](evidence/wp1-acceptance.md)） |
| REQ-02 | tokenizer一致 | 公式ids/mask fixtures | E1 | WP-2 | implemented | [独立比較](evidence/wp2-acceptance-parity.json)、[独立Goテスト](evidence/wp2-acceptance-go-test.log)、[実装報告](evidence/tokenizer-parity-worker.json) | pass（[独立受入](evidence/wp2-acceptance.md)） |
| REQ-03 | v4.1 ONNX graph | CPU/CUDA parityと欠損検査 | E1 | WP-1/3 | in_progress | [独立全graph](evidence/parity-acceptance.json)、[40step比較](evidence/parity-acceptance-shadow.json)、[I/O検査](evidence/acceptance-io-check.json) | WP-1部分pass（独立受入）。Go組込WP-3は未実施 |
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
- ユーザーによる試聴用4件の事前品質確認は完了（E4）。製品実装後の全要件受入は未実施。現在のWP-1受入とは区別する。

## 次の作業

WP-2のGo tokenizer parityは2026-09-08に独立受入pass。今回の依頼はWP-2のみのため、WP-3のmanifest/推論組込以降は未着手で引き渡す。WP-1の技術Goは製品移行完了ではなく、WP-4/5の設定・取消・品質・性能・E2Eまで通過する前に既定値を変更しない。





## WP-1 完了結果

- 受入: [正式報告](evidence/wp1-acceptance.md)。REQ-01 pass、REQ-03のWP-1 graph契約部分pass。REQ-03行のin_progressは後続WP-3が残るため。
- 最終manifest SHA256: `a5bec29d3787db2457d70d94693c2635c2ceefa091d0b1968af8665f8982060f`。metadata最終修正に対し全graph/重み/parity不変を [最終差分](evidence/acceptance-final-delta.json) で独立確認。
- 新規独立E1: 全6 graph CPU/CUDA parity、全graph動的長、speaker/codec境界、4条件、6外部重み欠損拒否、純ORT短文、40全step公式比較、公式duration差0、固定入力5破損拒否、新規隔離venvと代表再export。
- 純ORT WAV: 48kHz/mono、4.76秒（119frame）、全長10秒narrator、peak0.8374/RMS0.16027。音声: `model/irodori-v4.1-acceptance/ort-full-smoke.wav`。聴取品質合格や性能ベンチマークを意味しない。
- 解消事項: CUDA TF32誤差、codec動的padding分岐固定、hop誤記、公式speaker最小長/端数条件、PyTorch生成の誤代用、固定資産とライセンス表示。
- 保持する制限: 保存bundle約4.79GB（参照weight約3.44GB）。既存sidecarへ旧dataが残るため再作成は空出力先。RSS/VRAM観測はREQ-08定常性能判定へ拡張しない。
- 製品Goコードとv3資産・既定値は未変更。計画全体のstateはimplementingを維持。

## WP-2 完了結果（2026-09-08）

- REQ-02 pass。[独立受入](evidence/wp2-acceptance.md)で公式PretrainedTextTokenizerとの656正常条件のraw ids・全padded ids/mask一致、4無効長条件のエラー一致を確認。
- 変更前v3実装との656条件一致を独立確認。v4のPAD=3、metaspace、特殊token、byte fallback、BOSとtruncationを対応し、v3処理をlegacy分岐で維持。
- 独立E1: localtts/audiofmt/storeのGoテスト成功、skipなし。固定ローカルtokenizer資産がテスト実行に必要。全比較配列・再生成手順・hashは受入報告に保存。
- 今回はWP-2のみ完了。WP-3以降、v4製品推論・音声品質・性能・E2E・既定切替は未実施。追加の方式選択は不要、全計画stateはimplementingを維持。
