# 調査証拠と再現手順

基準: 2026-09-07 / アプリdb03bec8。実測概要と注意点は [調査資料](../../cheatsheet/irodori-v4-migration.md)。

## 前提

Windows RTX5090、既存 `model/irodori-v3/`、`narrator/narrator_01.wav`、GPU版ORT1.26.0とCUDA13 DLLがあること。v3の資産は既存READMEの導入手順に従う。ソース・Python環境・v4/v4.1の重みは調査用 `third_party/irodori-v4-research/` に分離する。

`reproduce.ps1` は今回個別に実行したコマンドを整理したラッパー。Setup一括の新規環境での再走は未実施。公式ソースとモデルrevisionは固定している。初回はネットワークと数GB単位の空き容量が必要。音声参照を外部へアップロードする処理はない。

PowerShellでリポジトリルートから実行:

```powershell
# ソースとtokenizer、Python3.11環境（今回の個別uv syncは成功済み）
& ./docs/plan_20260907_irodori_v4/evidence/reproduce.ps1 Setup
# Go tokenizerの比較元JSONとv3 CUDA WAV
& ./docs/plan_20260907_irodori_v4/evidence/reproduce.ps1 Tokenizer
& ./docs/plan_20260907_irodori_v4/evidence/reproduce.ps1 Baseline
# v4.1の5条件、v4初版の1条件。GPU測定は順次実行する
& ./docs/plan_20260907_irodori_v4/evidence/reproduce.ps1 V41
& ./docs/plan_20260907_irodori_v4/evidence/reproduce.ps1 V4
# v4.1実モデルで既存exporterの負例を再現
& ./docs/plan_20260907_irodori_v4/evidence/reproduce.ps1 Exporter
# 7 WAVのread-back検証
& ./docs/plan_20260907_irodori_v4/evidence/reproduce.ps1 Validate
```

生成物のJSON/WAVは同名で更新されるので、元の証拠を残したい場合は計画フォルダと出力を別途コピーしてから再実行する。今後のベンチマークでは測定run単位の出力先を設ける。

内部のPython実行形式は `mise x -- uv run --project <source> --python 3.11 --no-sync python <probe.py>`。Windows絵文字出力のため `PYTHONIOENCODING=utf-8` と `PYTHONUTF8=1` を設定する。公式ソースの.pyは改変していない。

## ファイルの意味

| 証拠 | 内容 / 判定範囲 |
| --- | --- |
| environment.json | GPU、ドライバー、ツール、参照音声/metadata/lockのhash |
| uv-sync.log | Python3.12でsentencepieceビルド失敗 |
| uv-sync-py311.log | Python3.11で公式CUDA依存の同期成功 |
| v4.1-runtime.log | 初回cp932の絵文字出力失敗。推論成功ログではない |
| v4.1-runtime-utf8.log / v4.1-runtime.json | 公式v4.1 5ケース成功、各段階時間・使用seed・WAV hash |
| v4-runtime.log / v4-runtime.json | 公式v4初版1ケース成功 |
| v3-baseline.log | 現行Go/CUDA短文生成成功。Service経由の文分割やUIの検証ではない |
| go-tokenizer.json / tokenizer-parity.json | Go/公式の6文比較。全件不一致が現行結果 |
| exporter-compat.log / exporter-compat.json | 実v4.1モデルで現行wrapperの失敗、speaker/duration specs欠落 |
| wav-validation.json | 7個の保存後PCM16を再読込した統計。runtime.jsonは保存前float統計なので丸め差がある |
| go-check.log | 現行audiofmt/store試験成功、localttsはテストファイルなし |
| go_probe/main.go | 既存パッケージだけを呼ぶ隔離Goスパイク。製品コード変更なし |
| runtime_probe.py | 公式SamplingRequestの実行、tokenizer比較、WAV/数値の保存 |
| exporter_probe.py | 実モデルをロードして既存TextEncoderModuleに渡す。期待する負例をassertするためexit0でもexport成功ではない |
| validate_wavs.py | 7 WAVを再読込し、PCM16/48k/mono/finite/非無音をassert |

## 再現性の限界

- 固定モデルrevisionを取得する一方、公式runtimeが内部取得するcodec/SilentCipher等の補助資産は上流の解決方法に従う。codecの実際のrevisionは実行ログにあり、補助モデル全体を完全固定した配布bundleはWP-1で作る。
- 同一v4.1 runtime内ではseed0のWAV hashが一致した。別マシン/ライブラリ/実装間のbitwise一致を保証しない。
- 速度はスモーク結果であり、統制されたp95/メモリベンチマークではない。
- 試聴用4件はユーザーが期待通りの音声品質と確認済み（[E4記録](user-audition.md)）。10原稿×3seedの聴取、ASR評価、Go v4.1 ONNX、CPU生成、E2E、BGM併用、全体buildは未実施。

最終検査でv4初版のローカルweightのhash不一致を確認した。`v4-corrupt-weight-run.*` は再取得前の異常実行（予測0.52秒）であり、成功/比較証拠に含めない。`model-integrity.json` に公式hashと観測値を保存した。runtime_probeは以後、推論前に固定weight hashを検査する。

## WP-1 実装・独立受入の証拠

WP-1は独立受入pass。正本は [wp1-acceptance.md](wp1-acceptance.md)、最終成果物契約は [wp1-manifest.json](wp1-manifest.json)。再作成は [exporter手順](../../../tools/irodori_export/README.md) に従う。

- `parity-acceptance.json` / log: CPU/CUDA全graph・動的長・境界・4条件・欠損拒否・純ORT短文。
- `acceptance_full_shadow.py` / `parity-acceptance-shadow.json`: 独立補完による40全stepと公式duration最終frame比較。
- `acceptance_fixed_inputs.py` / `acceptance-fixed-inputs.json`: 固定資産の正例と5破損拒否。
- `acceptance-isolated-sync.log`、`acceptance-reexport-parity.json`: 新規隔離環境と代表graph再export。
- `acceptance-final-delta.json`: 最終manifestと実行済みgraph/重み/parityの不変確認。
- `acceptance-external-storage.json`、`acceptance-gpu-memory.csv`: 保存容量と観測時メモリ。定常性能合格ではない。
- `wp1-final-snapshot.json` はライセンス最終訂正前の履歴。現行manifestは `wp1-manifest.json`（SHA256 a5bec29d...）を参照。

音声と数GBのgraphはignore対象。独立生成WAVは `model/irodori-v4.1-acceptance/ort-full-smoke.wav`。途中失敗ログの結果は最終合格へ混在させない。全アプリ回帰、Go v4組込、Wails E2E、品質/性能受入は今回の範囲外。
