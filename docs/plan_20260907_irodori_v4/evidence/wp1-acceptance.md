# WP-1 独立受入（中間報告）

担当: persistent acceptance。対象: REQ-01 / REQ-03のWP-1 graph契約部分。全計画/Goアプリ移行の完了判定ではない。
対象snapshot: da3ef30ae72f84491b0112ecebdc7280ce0aed12。code/bundle hashes: acceptance-target-hashes.json、差分: acceptance-target.diff。
実行対象manifest SHA256: 47dcc95d84df7e179404b134f63684c7854924ecc8019f1b6bd749d029cbe78b。

## 独立実行

- parity-acceptance.json/log: 2026-09-07 14:32:16–14:34:59 UTC、exit0。固定harnessを別model/irodori-v4.1-acceptanceで実行。worker JSON/WAVを上書きしない。
- parity-acceptance-shadow.json/log: 14:35:20–14:38:28 UTC、exit0。acceptance_full_shadow.pyは凍結parity.pyのコピーへDiT40全step oracleと公式duration最終frame比較だけ追加。製品コードは編集していない。40/40同入力比較成功。PyTorch最終119 frame、ORT119、差0。
- CPU/CUDA全6graph、全graph動的長、4条件のduration/DiT、speaker有効4/5/7/8/9/17・無効1/3、codec1919/1920/1921/479999/480000/480001、external data6欠損拒否は成功。
- 純ORT短文: 48kHz、4.76秒、119frame、参照全480000sample→250latent、40DiT、peak0.8374/RMS0.16027。参照normalize=None/ensure_max=False、trim0、SilentCipher前。NN全段ORT、CUDA実行profileあり。生成物はmodel/irodori-v4.1-acceptance/ort-full-smoke.wav。
- acceptance-io-check.json: 全graph実I/Oとmanifest一致、hash一致。
- acceptance-isolated-sync.log: 新規third_party/irodori-wp1-acceptance-venvへuv sync --locked --offline --python3.11成功。
- acceptance-reexport.log / acceptance-reexport-parity.json: 新規隔離環境からdurationを再exportし、別長ランダム入力で元graphと一致。全graphの再export反復は行っていない。

## 判定

REQ-03 WP-1部分: passを支持。WP-3（Go loader/pipeline実装）は未実施。REQ-03全体passとはしない。
REQ-01: 固定入力取得/照合とlicenseの修正delta待ち。現在の観測hashだけでは固定codec revision取得を保証しないこと等をleaderへ報告済。

## 容量と測定の限界

bundle4,794,293,787bytes。acceptance-external-storage.jsonによればcodec sidecarは参照値4倍、speaker2倍の未参照領域を含む。既存sidecarへのappendによりclean exportとの差がある。これは数値不一致ではないが再現手順でclean outputを要求する必要がある。
RSSは終了時約10,234.8MiB。torch allocatorはORT GPU使用量ではない。acceptance-gpu-memory.csvのnvidia-smiはホスト全GPUの観測点でありプロセス専用peakではない。性能・定常メモリREQ-08の証拠には拡張しない。

## 最終WP-1限定受入（2026-09-07、上の中間REQ-01保留を更新）

**REQ-01: pass。REQ-03のWP-1 graph契約部分: pass。WP-1: passを支持。**
WP-3およびREQ-03全体、WP-2以降、全計画doneは判定対象外。

最終manifestはwp1-manifest.json（SHA256 a5bec29d3787db2457d70d94693c2635c2ceefa091d0b1968af8665f8982060f）。wp1-final-snapshot.jsonは旧metadataの比較資料であり最終manifestの代用ではない。

追加の独立E1/E3:

- acceptance_fixed_inputs.py / acceptance-fixed-inputs.json: 固定source内容/archive/model/tokenizer/codecの正例受理と、各5種類の破損を期待SHA256エラーで拒否。codecはrevision47376ee24834d7a05a48ebabfe3cde29b3c5e214のsnapshotへ解決しfile SHA256 db120339c5ee7eca1912cdf29bc612b947a0808e69c3cebfb4936b45a762c1d5を確認。exit0。
- acceptance_final_delta.py / acceptance-final-delta.json: 最終manifest検証、6graphと全external dataのhash不変、parity.py不変、exporter/lock/mise記録hash一致、license原文hash一致を確認。exit0。最初の実行はWindows cp932でJSON読取失敗し、PYTHONUTF8=1/PYTHONIOENCODING=utf-8を指定した再実行が成功。
- source期待content/archive hash、model/tokenizer期待hash、codec固定revision取得→local file hash照合→公式loaderへのlocal file引渡しを静的確認。Python3.11.15/uv0.12.9の専用mise固定、uv.lockの固定DACVAE git依存を確認。
- DACVAEコード原文はApache-2.0、ModernBERT-ja原文はMIT。誤記修正後のexporter/manifest/READMEと一致。配布graph/tokenizer/外部dataと開発依存の区別、原文license参照/hashの記録を確認。
- READMEのroot cwd保持をuv --projectで実測確認。固定archive取得/hash確認/展開、空出力dir、export→externalize→merge→CPU+CUDA parity手順へ訂正済。新規venv構築と代表duration再exportの実行証拠を保持。全資産のネットワーク再downloadや全6再exportを繰返してはいない。

保持判断: 最終deltaは入力固定検査・license表記・再現手順・metadataのみ。独立成功済み6graph/外部重み/parity実装のhashは同じで、数値経路・入力fixture・CPU/CUDA条件は不変。したがって新たな重い全graph回帰は不要と判断し、標準E1と40step補完の結果を保持する。

docs/40_specification.mdのWP-1確定事項とdocs/cheatsheet/irodori-v4-migration.mdのWP-1追記は、hop1920/floor patch4/TF32無効/同入力40step/119frames差0/前後処理差/後続WP未実施という実測範囲と整合する。製品Goコード・設定既定値は対象外のまま。

残る事項は後続WPの実装と受入、配布時の同梱手順、空dir再生成による未参照sidecar領域の除去。現在の保存容量と参照容量の差は記録済みで数値不一致ではない。REQ-08の定常GPUメモリ・速度、REQ-09の聴取、REQ-10アプリE2Eを本WPの結果で代用しない。
