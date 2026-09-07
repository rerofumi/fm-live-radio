# Irodori v4 / v4.1 移行調査

確認日: 2026-09-07。調査担当: 本計画セッション。製品コードの移行は未実施。

## 対象を固定する

| 対象 | revision / 条件 |
| --- | --- |
| アプリ | db03bec8（開始時jj clean） |
| 現行モデル | Aratako/Irodori-TTS-500M-v3、metadata内revision 236c1e56591279fc24e3c1bf6609fc06e48dde28 |
| 公式推論 | Aratako/Irodori-TTS @ 8224dafb46d0aba89209a8f905f1cb7e3299d9c1 |
| v4初版 | Aratako/Irodori-TTS-v4-Small @ 4c92c7ee2bb15c19a97cf4e86d24fd6bf33b0135 |
| v4.1 | Aratako/Irodori-TTS-v4.1-Small @ 2b28324dc263ed5e6638b3cf3dd94c82ead07b4b |
| 現行exporter調査 | mtsmfm/Irodori-TTS-ONNX @ 5df35d8720f810902971745a5ad961ff436bd73c |
| exporterの推論依存 | mtsmfm/Irodori-TTS @ ac3274803ff1952d1cd03a052abed132fb756ce0 |
| 公式codec | Aratako/Semantic-DACVAE-Japanese-32dim @ 47376ee24834d7a05a48ebabfe3cde29b3c5e214（実行ログで解決先確認） |
| 実機 | Windows / RTX 5090 32607 MiB / driver 610.62 |
| 公式Python実測 | Python3.11.15 / torch2.10.0+cu128 / transformers5.12.1 / FP32 |
| 現行Go実測 | onnxruntime_go1.31.0 / ORT1.26.0 GPU CUDA13、既存DLLを使用 |

詳細は [environment.json](../plan_20260907_irodori_v4/evidence/environment.json)。`third_party/irodori-v4-research/` に公式ソース・独立venv・モデル・音声を配置した（既存gitignore対象）。それ以外にuv/HFの通常キャッシュが作成される。モデルは数GB単位なので、再現環境にはダウンロード容量とvenv容量を確保する。

## 公開情報の要点

v4はModernBERT-jaを用いた共有text/caption encoderとspeaker条件の統合、長い参照入力を持つ。v4.1は主にduration predictor更新の後続版。公式はv4.1を推奨する。短い単一参照では声の類似度がv3より低下する場合があり、現在のnarratorを使うアプリで自動的に品質向上するとは判断しない。[v4モデルカード](https://huggingface.co/Aratako/Irodori-TTS-v4-Small)、[v4.1モデルカード](https://huggingface.co/Aratako/Irodori-TTS-v4.1-Small)。

公開ベンチマークのv3比較対象は600M-v3-VoiceDesignであり、本アプリの500M-v3 baseとは異なる。quantization、120秒参照、絵文字品質は今回の採用根拠として実測していない。モデルカードのライセンス表記はMIT。配布時にコード・モデル・codec・依存の表示を個別に確認する。

## 採用理由と実行確認の対応

| 確認項目 | 最小検証 | 観測 | 判定 |
| --- | --- | --- | --- |
| 公式v4.1を既存参照でローカル生成 | SamplingRequest(ref_wav) | 48k/mono、非無音のWAV | 公式Python経路は実行可 |
| 自動発話長 | seconds=None、duration predictor | 120frame / 4.8秒などのログとWAV | 自動長経路は実行可、予測精度改善は未判定 |
| caption/speaker併用 | ref+caption、captionのみ | いずれも正常WAV | 参照+caption短文の品質はユーザー確認済み。captionのみ・意味追従の個別評価は未判定 |
| 同じseedの再現性 | 同一runtimeでseed0を2回 | 同一WAV SHA256 | この条件で一致、他環境の保証ではない |
| Go tokenizer流用 | 6文のids/mask比較 | 全件不一致 | 無修正流用No-Go |
| 既存exporter流用 | 実v4.1モデルにTextEncoderModuleを構築 | AttributeError、speaker/duration specsなし | 無修正流用No-Go |
| 改修後Go/ONNX | 未実施 | graph export・ORT生成・CPU/CUDA parity未確認 | 条件付き。WP-1で確認するまで採用Goではない |
| v3比の読み/声/性能 | 非同等backendの短文スモークのみ | 試聴4件は期待通りとユーザー確認。統計ベンチなし | 広範な比較・移行後出力は未判定、REQ-08/09へ |

[公式推論ソース](https://github.com/Aratako/Irodori-TTS/tree/8224dafb46d0aba89209a8f905f1cb7e3299d9c1)、[exporterソース](https://github.com/mtsmfm/Irodori-TTS-ONNX/tree/5df35d8720f810902971745a5ad961ff436bd73c)。実行したAPI名と引数は [runtime_probe.py](../plan_20260907_irodori_v4/evidence/runtime_probe.py) に保存。

## 実測一覧

共通の比較文:「こんにちは。今日はラジオの音声合成を確認します。」。既存 `narrator/narrator_01.wav` は48kHz、10.0秒。40 steps、seed0、text/caption/speaker CFG=3/3/5。

| 経路 / ケース | ロード秒 | 合成秒 | WAV秒 | RTF | PCM16 RMS |
| --- | ---: | ---: | ---: | ---: | ---: |
| v3 Go/CUDA、参照あり | 6.166 | 3.028 | 4.80 | 0.631 | 0.14951 |
| v4公式CUDA、参照あり | 15.873 | 5.100 | 4.84 | 1.054 | 0.16091 |
| v4.1公式CUDA、参照あり初回 | 23.632 | 4.065 | 4.80 | 0.847 | 0.16084 |
| v4.1公式CUDA、同条件warm | 同runtime | 3.892 | 4.80 | 0.811 | 0.16084 |
| v4.1公式CUDA、ニュース文(seed1) | 同runtime | 3.983 | 5.88 | 0.677 | 0.16592 |
| v4.1公式CUDA、参照+caption | 同runtime | 3.888 | 4.72 | 0.824 | 0.16096 |
| v4.1公式CUDA、captionのみ | 同runtime | 3.836 | 5.00 | 0.767 | 0.16171 |

RTF=合成時間/音声時間。合成時間はロードを除く。**性能優劣の比較表ではない**。v3はGo/ORT、v4系はPyTorch、参照音量正規化・乱数・CFG時刻・tail trim・SilentCipherが異なる。v4行はweight再取得後の再実行値。測定は別時刻で、ホスト負荷やキャッシュを統制していない。ロード時間には初回補助モデル準備を含み得る。v4.1 warmも1回でありp95ではない。

v4.1全5ケースでPyTorch allocated最大4423.92 MiB、reserved最大5048 MiB（プロセス内の累積peak）。これは総GPU使用量でも、将来のONNXメモリ見積りでもない。全7 WAVを再読込し48kHz/mono/PCM16/finite/非無音を確認した。[WAV検証](../plan_20260907_irodori_v4/evidence/wav-validation.json)、[v4.1実測](../plan_20260907_irodori_v4/evidence/v4.1-runtime.json)、[v4実測](../plan_20260907_irodori_v4/evidence/v4-runtime.json)、[v3ログ](../plan_20260907_irodori_v4/evidence/v3-baseline.log)。

## 互換性の具体的な落とし穴

1. 同じUnigramでもtokenizer互換とは限らない。v4のPADは3だが現行Goは4にfallbackする。Goは先頭metaspaceを追加するため、日本語短文のBOS直後に余分なID271が入った。英数、絵文字、異体字、空白を含む全6件で不一致。[比較JSON](../plan_20260907_irodori_v4/evidence/tokenizer-parity.json)。
2. 既存exporterのTextEncoderModuleは `model.text_encoder.text_embedding` を要求する。実v4.1はPretrainedConditionProjectorなので構築失敗。export_specs_forもcaption有効時にspeaker/durationを選ばない。[実行結果](../plan_20260907_irodori_v4/evidence/exporter-compat.json)。これはwrapperに実モデルを与えた検証であり、既存lockのCLIによるフルexport試験ではない。負例再現をassertするスクリプトのexit0をexport成功と読まない。
3. v4.1実モデルconfigはspeaker_patch_size=4、duration_architecture=token_sum_dual_adarn_zero_no_aux、caption/speaker両方有効。Goの排他分岐・参照長固定・旧duration I/Oを流用しない。
4. 公式で生成したWAVにはSilentCipher処理が実行された。透かしをdecodeして真正性を検証したわけではない。処理中の44.1kHz変換warningが出るが、最終出力は48kHzだった。ONNXとの比較点を揃える必要がある。
5. モデルgeneration上限とreference上限は異なる。公式SamplingRequestの生成上限は30秒、参照はcheckpoint推奨上限。長いニュースはアプリ側の分割が必要。

## 再現と環境失敗

[再現手順](../plan_20260907_irodori_v4/evidence/README.md) と保存したスパイクを使用する。製品のmise.toml/go.mod/モデル設定は変更していない。

- `mise install`: 成功。Pythonの実行は `mise x -- uv ...` に統一。
- 公式 `uv sync --extra cu128 --python 3.12`: sentencepiece0.1.99のソースビルドでCMake最小バージョン互換エラー。
- `--python 3.11`: 成功。torch2.10.0+cu128を導入。Python3.12への依存変更は今回行っていない。
- Windows標準cp932のstdoutは絵文字でUnicodeEncodeError。`PYTHONIOENCODING=utf-8` と `PYTHONUTF8=1` で再実行成功。
- `mise x -- go test ./internal/localtts/... ./internal/audiofmt/... ./internal/store/...`: 成功。ただしlocaltts各packageにはtestファイルなし。これはTTS音質テストの成功を意味しない。

更新条件: モデル/公式ソース/exporter/ORT/transformers/tokenizerのrevision変更、GPU変更、参照音声変更時は対応する検証を再実行する。

## 補足: ローカルweight破損を検出した例

最終検査でv4初版のJSON/ログにNUL bytesが見つかり、再実行したところ出力が0.52秒へ短縮した。weightのファイルサイズは正常だがSHA256が公式LFS値と異なっていたため、この実行を比較から除外した。原因は未確定。v4.1 weightと既存narratorはhash一致。[hash証拠](../plan_20260907_irodori_v4/evidence/model-integrity.json)。

再取得したweightで確認をやり直し、runtime_probeに推論前hash検査を追加した。モデルがロードでき、WAVが非無音であっても破損を否定できないため、配布bundleは存在/サイズの検査だけにしない。




## 試聴確認（2026-09-07追記）

ユーザーがv3、v4、v4.1、v4.1参照+captionの4サンプルを試聴し、すべて期待通りの音声品質と報告した。[対象とhash・評価範囲](../plan_20260907_irodori_v4/evidence/user-audition.md)。v4.1の短文品質に関する事前確認として利用できるが、モデル間の優劣や改修後Go/ONNXの品質保証には拡張しない。
## WP-1 ONNX実測（2026-09-07）

開発用exporterは `tools/irodori_export/`、成果物は `model/irodori-v4.1/`。製品Go推論はまだv3。独立受入でtext/caption共有encoder、speaker、dual duration、DiT step、codec encoder/decoderの6 graphがCPU/CUDAで基準内（atol=1e-4、rtol=1e-3）と確認された。[独立受入](../plan_20260907_irodori_v4/evidence/wp1-acceptance.md)。

- CUDAはPyTorch/ORT双方でTF32を無効にする。初回text graphの誤差5.47e-4は設定後1.19e-6へ改善。provider名だけでなくprofilingでCUDAノード実行を確認し、shape用CPUノードは区別する。
- speaker patch=4はfloor/truncate。最小正常長4、1/3は公式も拒否する。ceil paddingへ変えると端数時の条件長が変わるため不可。4/5/7/8/9/17をCPU/CUDAで検証。
- DACVAE hopは1920 samples（48kHz）。公式の長さ依存padding分岐をtraceしただけではhop倍数にも余分なpaddingが付く。動的reflect paddingを表現し、1919/1920/1921、479999/480000/480001をCPU/CUDA比較する。480000 samplesは250 latent。参照を1 sample削って不一致を隠さない。
- 純ORT生成は10秒narrator、40step、全NNをORT実行し4.76秒/119frameのWAVを生成。独立補完probeで40全stepを同じ入力の公式出力と比較し基準内、公式durationとの差0。透かし前の数値比較であり、聴取品質・Go組込・実時間性能の合格ではない。
- 計測時の参照条件はmax_ref_seconds=120、normalize=None、ensure_max=False。今後のGo側前後処理はこの条件と公式既定の差を明示して扱う。
- 外部dataは全graphで欠損を拒否。再exportでsidecarに未参照旧dataが残ることがあるため、再作成は空ディレクトリへ行う。受入時の保存サイズ約4.79GBと参照weight約3.44GBを混同しない。

証拠: [全graph](../plan_20260907_irodori_v4/evidence/parity-acceptance.json)、[40step/公式duration](../plan_20260907_irodori_v4/evidence/parity-acceptance-shadow.json)、[隔離再export比較](../plan_20260907_irodori_v4/evidence/acceptance-reexport-parity.json)。固定環境・配布対象の最終状態は計画90_statusとexporter READMEを参照する。

## WP-2 tokenizer実測（2026-09-08）

固定v4.1 tokenizer（SHA256 6a0734cf…）と公式PretrainedTextTokenizerに対してGoを修正。v4はnormalizerなし、Metaspace prepend_scheme=never/split=false、PAD=3。入力内PAD特殊tokenはactive mask=true。BOSは長さ上限に含める。Unicodeを無条件に正規化しない。UTF-8 fallbackはbyte単位で経路を扱い、v4はfloat64スコア、v3は旧float32/旧fallbackのlegacy処理を維持する。

独立比較: v4全ids/mask/raw ids 656条件一致、無効長4条件一致、変更前v3との656条件一致。Go限定回帰はskipなしで成功。通常go testにはローカルmodel/irodori-v3・irodori-v4.1のtokenizer資産が必要で、欠損は失敗する。[環境・hash・再生成手順・全配列](../plan_20260907_irodori_v4/evidence/wp2-acceptance.md)。固定資産の保証であり汎用HF tokenizer対応ではない。資産/source/依存またはtokenizerコード変更時は同じ比較を再実行する。製品v4推論は後続WP-3。
