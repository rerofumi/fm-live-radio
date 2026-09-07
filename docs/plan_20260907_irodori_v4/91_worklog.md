# 計画作業ログ

worklog: enabled

## WL-001 — Step 0: 計画開始（2026-09-07、調査セッション）

- ユーザー確認: Full、E2E 検証計画、作業ログあり。
- 入力: 現行コード、model/irodori-v3/metadata.json、既存計画の Claim、jj status。
- 結果: 作業開始時は clean。現行は 500M-v3 の Go/ONNX 推論。既存計画はローカル推論導入、CUDA、UI 等であり、今回の v4 世代移行は新規計画とする。
- 方針: 公式 v4/v4.1 を調査し、隔離ディレクトリで検証する。移行実装は今回の範囲外。
- 未確定: Go/ONNX の対応方法と実測品質。次に公式ソースと tokenizer 契約を確認する。

## WL-002 — Steps 1–2: 移行目的と範囲（2026-09-07）

- 入力: ユーザー依頼、requirement.md、specification.md、localtts.Service と保存設定。
- 決定: ラジオの日本語原稿を既存 narrator で生成する用途を最小範囲とする。v4.1 は v4 の後続候補として評価。caption UI、複数参照 UI、量子化は初回切替の必須条件にしない。
- 理由: 現行アプリは 500M-v3 base を使用。公式ベンチマークの v3 VoiceDesign と同一モデルではなく、現行品質の改善は別途比較が必要。
- 次: 調査証拠を作り、採用前提と未確定事項を分けて要求へ反映する。

## WL-003 — Steps 3–4: ドメイン確認と実行調査（2026-09-07）

- 入力: 公式 v4/v4.1 モデルカード、固定 revision の推論ソース、mtsmfm exporter、現行 metadata/tokenizer/pipeline。
- 確認: 120 秒は参照音声の上限であり、生成上限ではない。v4 は caption/speaker の同時条件と speaker patch=4。現行は排他的分岐と speaker patch=1。
- 実行: mise install 成功。Python 3.12 の sentencepiece ビルド失敗を保存。Python 3.11 で公式 cu128 依存導入成功。現行 v3 CUDA baseline 成功（ロード 6.166 秒、合成 3.028 秒）。
- 成果物: evidence/go_probe/main.go、v3-baseline.log、uv-sync*.log、runtime_probe.py。
- 未確定: 公式 v4.1 の実生成、tokenizer parity、ONNX export の適合。続く実測で解消する。

## WL-004 — Step 4: 実測完了と方針確認（2026-09-07）

- 実行: v4初版1件、v4.1 5件、v3 1件のWAV生成。7件を再読込しPCM16/48k/mono/finite/非無音を確認。
- 結果: v4.1は同一seed再実行のWAV hash一致。tokenizer6文すべて不一致。実モデルでTextEncoderModuleが失敗、specsのspeaker/duration欠落を確認。
- 訂正: スパイク初回のTextEncoderWrapperは調査コード側のクラス名誤りであり製品不具合ではない。TextEncoderModuleへ修正して非互換を再現した。
- ユーザー決定: Go/ONNX維持・v4.1第一候補。Pythonサーバーは採用しない。
- 制約: 品質聴取、対応ONNX、アプリE2Eは未実施。速度のcross-backend優劣は主張しない。

## WL-005 — Steps 5–6: 要件と仕様（2026-09-07）

- 入力: 実行ログ、tokenizer-parity、exporter-compat、現行service/pipeline/store。
- 成果物: 10_claim、20_app_requirement、30_requirement、40_specification。
- 決定: REQ-01–10、WP-1–5、ONNX実現性ゲート→明示試験運用→受入→新規既定変更の順序を定義。
- 理由: 現行の非互換を先に解消し、既存利用者の設定を維持する。
- 次: 調査cheatsheet、review/status/indexを整合させ、未実施項目を残して計画を引き渡す。

## WL-006 — Step 7: 調査・レビュー記録・状態（2026-09-07）

- 成果物: cheatsheet/irodori-v4-migration、domain_primer、50_review_notes、90_status、plan_index。
- 決定: 調査成功は実装成功に転記しない。REQ-01–10をtodo、acceptance未実施、state=plannedとした。
- 根拠: 改修版ONNXは未生成、聴取・CPU・アプリE2Eも未実施。
- 次: レビュー入口と再現手順を確認する。

## WL-007 — Step 8: 引き渡し資料（2026-09-07）

- 成果物: 60_review_packet。利用者の変化→確認済み方針→実測→実施順→未確認→根拠の順で整理。
- 判断: 追加の方針確認は不要。Go/No-GoはWP-1の技術検証に委ねる。
- 範囲外: Steps9–11（製品実装・独立受入・as-built更新）は今回実施しない。
- 次: 文書リンク/証拠/変更範囲の最終確認を行い、計画として渡す。

## WL-008 — 最終検査中の証拠整合性対応（2026-09-07）

- 現象: v4初版の結果JSON/ログがNUL bytesになっていることを検知。v4再実行では予測長6.9frames→0.52秒のWAVとなった。
- 検証: ローカルv4 weightのSHA256が公式LFS hashと不一致。v4.1 weightと既存narratorは一致。原因と中断の関係は断定していない。
- 措置: 破損weightでの実行はevidence/v4-corrupt-weight-run.*として残し、有効な品質/性能比較から除外。公式固定revisionを再取得し、hash一致を確認して再実行する。
- 改善: runtime_probeに推論前hash検査を追加。REQ-06と仕様にサイズを保ったweight破損の検出を明記。
- 最終検査: v4再実行中ファイル以外のJSON/ソース/文書はNUL無し、JSON parseとローカルリンク・Python AST成功。

## WL-009 — 再検証と計画確定（2026-09-07）

- v4の再取得後SHA256は公式と一致。再生成は121frames / 4.84秒、WAV hashは当初観測値14feb386…と一致した。ロード15.873秒、合成5.100秒へ実測表を更新。
- 破損weightの0.52秒結果は負例として別保存し、比較表へ入れていない。
- 再現ラッパーはV4アクションを実際に実行。Setup一括の新規環境再走は未実施と明記。
- 最終確認対象: 全JSONのparse/NUL検査、ローカルリンク、スクリプト構文、REQ対応、WAV hash一致、jj差分がdocsのみであること。

## WL-010 — ユーザー試聴の反映（2026-09-07）

- 入力: 試聴用4件すべてが期待通りの音声品質というユーザー報告。
- 証拠: evidence/user-audition.md。4 WAVのSHA256を既存検証記録と照合し一致。
- 更新: 要件補足、レビュー資料/記録、status、調査cheatsheet、証拠READMEを更新。
- 判定: 4件の事前品質確認は完了。10原稿×3seedと移行後Go/ONNX評価は未実施のためREQ-09全体は未完了。実装には着手していない。
## WL-011 — Step 9: WP-1 実施開始（2026-09-07）

- 入力: ユーザーのWP-1実施依頼、30/40/90、調査cheatsheet、既存再現手順。開始時jj clean、parent d8358ef5、working snapshot efe0480b。
- 範囲: 固定exporter環境と全ONNX graph、CPU/CUDA parity、短文ORT生成による技術Go判定。WP-2以降の製品実装と既定切替は対象外。
- 体制: fm-dev-implementに従い、新規workerと独立acceptanceを分離。検証範囲はexporterと生成成果物に限定し、アプリの未変更領域の全回帰は今回要求しない。
- 環境: 通常sandboxでは指定Eドライブへのアクセス拒否を確認。権限昇格したPowerShellによる指定リポジトリの読み取りは成功。
- 次: 全graphの実測証拠を収集し、独立受入で固定成果物と移行可能性を判定する。
## WL-012 — Steps 9–10: WP-1 初回実装・中間レビュー（2026-09-07）

- 成果: tools/irodori_export と隔離model/irodori-v4.1に6 graph、外部data、manifestを作成。
- 独立調査: 数値比較/終了判定、依存固定、動的/条件比較、短文生成の不足を検出。CUDA精度差は未解消。
- 判断: 初回workerの成功報告を受入へ転記せず、50_review_notesに未達を記録。新規fix workerへ厳密な検証・CUDA切分け・公式生成ループを委譲。
- 次: 修正後の成果物hashに対応する独立E1を実施。全計画はWP-2以降が残るためimplementingを維持。
## WL-013 — Step 10: WP-1 graph独立受入（2026-09-07）

- 独立担当が別bundle/別出力でCPU/CUDA全6 graph、動的長、speaker/codec境界、4条件、外部data欠損、実ORT短文を再実行し成功。
- 補完probeで40全stepの同一入力PyTorch/ORT比較を実行し全一致基準内。公式durationとORTは119 frame、差0。
- 証拠: evidence/parity-acceptance.json、parity-acceptance-shadow.json、acceptance-io-check.json。
- 判定: REQ-03のWP-1 graph契約部分pass。Go組込WP-3は未実施。REQ-01は隔離venv構築成功、固定資産/再現/ライセンスの受入を継続。
## WL-014 — Steps 10–11: WP-1受入完了と文書反映

- 独立担当の正式受入: REQ-01 pass、REQ-03 WP-1部分pass。固定入力5破損拒否、新規隔離環境/代表再export、全CPU/CUDAと40step比較を確認。
- 最終metadata: DACVAEコードApache-2.0を原文と一致させ、取得時cwd/固定archive手順を訂正。graph/重み/parity不変により成功証拠を保持。
- 文書: 90_status、plan_index、40仕様、50/60、cheatsheetと証拠READMEへ反映。製品仕様はv3のまま、WP-1を製品移行済みとは書かない。
- 結果: WP-1完了・技術Go。WP-2以降が未実施のため計画stateはimplementing。次の担当はGo tokenizer/推論組込へ進む。

## WL-015 — Step 9: WP-2限定実施開始（2026-09-08）

- 入力: ユーザーのWP-2のみの実施依頼、REQ-02、WP-2仕様、既存tokenizer比較とWP-1結果。
- 範囲: 公式PretrainedTextTokenizerとのids/mask一致、境界fixtures、v3 token列の保持。WP-3以降は今回実施しない。
- 体制: 新規workerと独立受入を分離。tokenizerと直接利用箇所のtargeted検証を行う。
- 環境: sandboxでEドライブへのアクセス拒否を再確認し、対象限定の権限昇格読み取りは成功。
- 判断: 汎用bindingの新規採用など既存仕様で決まらない事項は、調査根拠を揃えて利用者へ返す。

## WL-016 — Step 9: WP-2 tokenizer実装（2026-09-08）

- 実装: `internal/localtts/irodori/tokenizer` を v3/v4 設定別に拡張。v4 の Metaspace `prepend_scheme=never`、`<pad>`=3、literal added token、UTF-8 byte fallback、float64 Unigram DPを反映。v3は旧DP/正規化を `legacyV3` として保持。
- fixtures: 公式固定 tokenizer SHA256 `6a0734cf21c802169defaffe719bc2ef12bb9d0be37e54b61ed27aa89394723d` 由来の既存6文、空/空白/改行、特殊token、複数byte、ASCII byte長255/256/257、token数255/256/257、およびv3 goldenを追加。
- 検証: `mise x -- go test ./internal/localtts/irodori/tokenizer -count=1` 成功。直接consumerを含む `mise x -- go test ./internal/localtts/... ./internal/audiofmt/... ./internal/store/...` 成功。[worker証拠](evidence/tokenizer-parity-worker.json)
- 範囲: REQ-02 implementer evidenceのみ更新。独立受入、受入欄、計画state、WP-3以降、v4既定切替は未実施。

## WL-016 — Step 9–10: WP-2実装報告と独立受入開始（2026-09-08）

- worker報告: tokenizerのv4設定解釈・特殊token・UTF-8 byte fallback・精度を修正し、v3をlegacy分岐で保持。境界テストとworker証拠を追加。
- 中間レビュー: v3共通処理への影響、byte数とtoken数の境界混同、fixture参照パス、正解系の出所確認を指摘しworkerへ共有。
- 検証報告: tokenizerとlocaltts/audiofmt/storeのGoテスト成功。証拠はevidence/tokenizer-parity-worker.json。
- 次: 同じ独立受入担当が完成差分と正解系を確認しE1を再実行。worker報告だけでREQ-02をpassにしない。後続WPが残るため計画stateはimplementingを維持。

## WL-017 — Steps 10–11: WP-2独立受入完了（2026-09-08）

- 独立受入: REQ-02 pass。公式656正常条件＋4無効長、旧v3の656条件を直接比較。Go回帰はskipなし成功。全配列・固定source/asset/hashと手順はevidence/wp2-acceptance.mdに集約。
- 訂正: WL-016のfixture参照パス指摘は受入担当の階層数の数え違いで撤回。v3分離・token境界・公式provenanceは完成版で確認済み。
- 反映: 90_status、plan_index、40仕様、60レビュー、cheatsheet、現行specificationを更新。計画全体はimplementing維持。
- 判断: WP-2に新binding採用等の追加方式選択は不要。ユーザー指定のWP-2だけで終了し、WP-3以降は未着手として引き渡す。
