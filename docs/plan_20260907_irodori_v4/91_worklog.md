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

## WL-018 — Steps 9–11: WP-3 Go/ONNX実装（2026-09-08）

- 実装: `metadata.Manifest` にschema-v2の6 graphの固定名・I/O名・dtype・shape・external dataサイズ/存在を追加。`pipeline/v4.go` を新設し、text/caption共有ModernBERT、speaker patch=4（floor/truncate、mask、masked summary）、dual duration features、独立caption/speaker null条件、0.5–1 CFG、named tensor、codec encoder/decoderを実装。`metadata.json` のv3経路はlegacyのまま保持。
- 音声契約: v4参照はnormalizeなし、resample後の不完全codec tail trim、SilentCipherなしを固定。decoder出力のfinite/non-silentを検査して48 kHz mono PCM16 WAVへ出力。
- task/test: `cmd/tts-parity` と `mise run tts-parity` を追加し、model/EP/seed/fixtureを明示可能なCLIを実装。metadata/pipeline/sampler契約テストを追加。
- 検証: expanded `mise x -- go test ./internal/localtts/... ./internal/audiofmt/... ./internal/store/... -count=1` 成功、固定CPU task成功、ORT 1.26 CPUによる1 step/0.5秒短文smoke成功（全6 graph load、49,964 byte WAV）に加え、`seconds=-1` のduration predictor経由smokeも成功（96,044 byte WAV）。いずれもfinite/non-silent検査後に一時ファイル削除。
- 未実施: CUDA Go/ORT、4条件の独立graph比較、WP-4のサービス設定/取消/hash preflight、WP-5の性能/品質/E2E/既定値切替。証拠: [WP-3 worker report](evidence/wp3-worker.json)。

## WL-019 — Step 10: WP-3初回独立受入（2026-09-08）

- 独立E1でexpanded Goテスト、BGM直接consumer、CPU 40step/自動duration生成は成功。WP-1/2の固定証拠はhash不変により保持。
- `tts-parity`がfixture値を比較せず任意入力でも成功すること、CUDA指定がCPU DLL固定で失敗することを確認。公式との差として参照波形のcodec前切捨て、v4のsway時刻列、不完全なnormalization/duration features、seconds crop未実装を検出。
- 判定: WP-3 fail。REQ-03〜05をin_progressとし、Acceptanceをfailへ更新。詳細は [初回独立受入](evidence/wp3-acceptance-round1.md)。計画stateはimplementingを維持し、新規fix workerへ修正を委譲する。

## WL-020 — Step 10: WP-3 fix1再受入（2026-09-08）

- fix1と独立E1でCPU/CUDAの6 graph・4条件・cfg=1/既定・40step数値比較、0.5秒crop、自動duration実生成、v3/CUDA、全Go test、frontend/Wails buildが成功。
- 残件: 製品consumerのnormalized text trim、製品pipelineへの同一fixture注入、fixtureの固定revision/再生成/配布E3、正しいmanifest負例。独自parity loopの成功を製品runtime parityへ拡張しない。
- 判定: WP-3は引き続きfail/in_progress。詳細は [fix1独立受入](evidence/wp3-acceptance-round2.md)。新規fix workerへ残件を委譲する。

## WL-021 — Step 10: WP-3 fix2再受入（2026-09-08）

- normalize+trim、製品CFG/DiT/sampler/codec経路共有、固定fixture再生成、metadata負例は独立確認。CPU/CUDA parityとseconds上下限の実WAVも成功。
- fixture非同梱の製品bundleが通常LoadManifestで失敗し、製品duration/外側loopはparity専用実装と分離したままでdurationScale・最終frameを証明できない。
- 判定: WP-3は引き続きfail/in_progress。詳細は [fix2独立受入](evidence/wp3-acceptance-round3.md)。新規fix workerへ残る2点を委譲する。

## WL-022 — Step 10: WP-3 fix3再受入（2026-09-08）

- fixture非同梱bundle load、parity専用fixture検証、製品/parity共通denoising、CPU/CUDA数値比較を独立確認。REQ-03のWP-1/3をpass。
- 残件: durationScale smoke配線、製品raw duration共有、非ゼロ14 featuresと公式Scale別最終frame、fixture-case報告条件数。
- 判定: REQ-04/05はin_progress、計画stateはimplementing。詳細は [fix3独立受入](evidence/wp3-acceptance-round4.md)。

## WL-020 — Steps 9–11: WP-3初回fail修正（2026-09-08）

- 対象: REQ-03/04/05のWP-3部分のみ。初回受入で指摘されたmanifest転記だけのparity、CPU DLL固定、codec前切捨て、sway時刻列、normalization/duration feature差、seconds cropを修正。WP-4/5とAcceptance/state欄は変更していない。
- parity: 固定公式source/checkpointから `tools/irodori_export/generate_go_fixture.py` で値付きfixtureを生成（`model/irodori-v4.1/go-parity-fixture.json`、SHA256 `e584c1a48244d33819d32ea8002c08e370b0669b14bbec5a1436e55c34a01e45`）。Go harnessは全6 graphの実入力/期待値、4条件、cfg=1/default、linear 40stepをatol=1e-4/rtol=1e-3で比較し、不正fixture/seed/EPは失敗する。
- 実行: `mise run tts-parity` CPU pass、`mise x -- go run ./cmd/tts-parity --model model/irodori-v4.1 --ep cuda --seed 0` CUDA pass。CUDA DLLを明示し、ORT CUDA `use_tf32=0`。CPU/CUDAは別プロセスで実行した。
- runtime: v4 linear 0.999 schedule、公式text normalization（括弧/[n]/番号記号/波線/連続ピリオド/Unicode）、caption trim-only、duration全14要素（emoji/補助漢字含む）へ一致。参照全波形をdynamic-padding codecへ渡し、latent後にpatch=4 floor/truncate。明示secondsはdecoder後cropし、0.5秒WAVは48044 bytes=24000 samplesでCPU/CUDA smoke pass。
- 検証: `mise x -- go test ./internal/localtts/irodori/... ./internal/generation/... ./cmd/tts-parity -count=1` pass。参照hop±1/4,5,7,8,9,17境界および1/3拒否、normalization/duration Unicode契約テストを含む。
- 追加検証: expanded `mise x -- go test ./internal/localtts/... ./internal/generation/... ./internal/audiofmt/... ./internal/store/... ./internal/musicgen/... -count=1` pass。最終helper変更後のCUDA 40-step数値parityもpass（6 graph/4条件、atol=1e-4/rtol=1e-3）。
- 未実施: WP-4サービス統合/取消/hash preflight、WP-5性能/品質/E2E/既定値切替。Acceptance判定は独立受入担当へ引き継ぐ。

## WL-021 — Steps 9–11: WP-3 fix2 残件修正（2026-09-08）

- trim: `normalizeV4SynthesisText` を製品v4のtext境界にし、正規化後trim済み文字列をtokenizerとdurationへ渡す。captionは従来どおりtrim-only。実tokenizer ids/maskと14要素duration featureが前後空白で同一になるconsumer testを追加。
- parity: 独自CLIのEuler/直接session比較を廃止し、`pipeline.CompareV4Fixture` から製品v4の `runTextCaption`、`runSpeaker`、duration raw、codec encoder/decoder、`runDiTBatch`、CFG、linear schedule、samplerを実行。6 graph・4条件・cfg=1/default・40stepの同一公式fixtureを比較する。
- provenance: 固定source `8224daf...d9c1`、model `2b28324...7b4b`、tokenizer `77675f...4ec6`、codec weights revision `47376e...e214` / code `414c20...a6e`、uv.lock/generator hashをfixtureへ記録。codec weightsは固定hashを検証してloadし、空の隔離出力から再生成して primary/rebuild SHA256 `b872cbe9...e14b7b` が一致。
- metadata: manifest graph `bytes`/`sha256` と fixture `sha256`を契約必須化し、`VerifyHashes` のgraph/fixture空hash skipを除去。正常manifestから `dit_step` input名だけを壊す負例が契約エラーになることを確認。VerifyHashes/graph preflight呼出し自体はWP-4責務として実施していない。
- 検証: targeted package+CLI、指定expanded suite、`go test ./...`、CPU/CUDA product parity pass。0.5秒smoke WAVは48kHz mono PCM16 24000 samples、finite/non-silent（48044 bytes、peak 32734、RMS 5491.01）を確認したが、PowerShell wrapperはORTログ後に最終JSON/exit行を返さなかったため、その点は証拠に明記。
- 未実施: WP-4サービス統合/取消/hash preflight呼出し、WP-5性能/品質/E2E/既定値切替。Acceptance欄とplan stateは変更していない。

## WL-023 — Steps 9–11: WP-3 fix4 実装（2026-09-08）

- 対象: WP-3の残件3点のみ。`cmd/tts-parity` の `--duration-scale` を smoke の製品 `pipeline.Options.DurationScale` へ配線し、seconds=-1・同一入力/seedでscale 0.5/1/2の自動WAVを `24960/48000/94080 samples`（0.52/1.00/1.96秒）として確認。明示seconds=0.5はscale=2でも24000 samples/48044 bytes。
- 共通化: 製品 `resolveDuration` と parity fixture の raw duration predictor tensor構築/実行を `v4Runtime.runDurationRaw` に集約。公式 `build_duration_features` で日本語・かな・数字・記号・emoji・補助CJKを含む14要素を生成し、fixtureへrawとScale 0.5/1/2の公式最終frame期待値を保存。CPU/CUDAとも全14要素、raw、expm1/scale/round/clamp、最終frame差<=1を比較。
- CLI: `--fixture-case reference` 実行時のJSON `conditions` を1へ修正（allは4）。fixture再生成SHA256は `57a34f21518197200776159d0e2770893067d37ee7e8c73603ad07ed17dd9039`、manifestも更新。証拠: [WP-3 fix4 worker](evidence/wp3-fix4-worker.json)。
- 検証: targeted Go test、CPU/CUDA product parity、fixture独立再生成、auto/explicit smoke、`mise x -- go test ./... -count=1` が成功。WP-4/5とAcceptance/state欄は変更していない。

## WL-025 — Step 10: WP-4初回独立受入（2026-09-08）

- service CPU/CUDA/auto、複数文、300ms文間、3秒失敗文、全失敗拒否、preflight負例、v3、buildを独立確認。
- 公開`Synthesize()`のOptions上書き回帰、direct smoke取消誤判定、app/musicgen終了時のcancel/join/Close寿命不足を検出。
- 判定: WP-4 fail、計画stateはimplementing。詳細は [WP-4初回独立受入](evidence/wp4-acceptance-round1.md)。fresh fix workerへ委譲する。

## WL-026 — Step 10: WP-4 fix2独立受入（2026-09-08）

- publish gate、実lifecycle、cancel/join/Close/再生成、並行Close実装とfull regressionを独立確認。
- `--expect-error`が検証不変条件違反までexit0にするCLI誤成功を検出。Closeテスト開始barrierも補強が必要。
- 判定: WP-4は検証CLI修正待ち。詳細は [WP-4 fix2独立受入](evidence/wp4-acceptance-round3.md)。

## WL-026 — Step 10: WP-4 fix1再受入（2026-09-08）

- 公開APIとdirect smoke取消を独立確認しREQ-05 pass。full Go/frontend/Wails、CPU parity、CPU/CUDA/auto複数文を確認。
- 残件: Player取消後publish競合、固定値でない実in-flightイベント、active Talk/BGM/prefetch終了、並行Close保証。
- 判定: WP-4はREQ-07により未完了。詳細は [WP-4 fix1独立受入](evidence/wp4-acceptance-round2.md)。fresh fix workerへ委譲する。

## WL-024 — Step 10: WP-3最終独立受入（2026-09-08）

- fix5で公式fixtureの入力条件から製品`buildDurationFeatures`を実行し、4条件×14値を直接比較。期待値の1要素改竄拒否、CPU/CUDA、fixture再生成を独立確認。
- 判定: WP-3 pass。REQ-03/04 pass、REQ-05のWP-3技術部分pass。詳細は [WP-3最終独立受入](evidence/wp3-acceptance-final.md)。
- 次: 計画stateはimplementingを維持し、設定・hash preflight・複数文・取消・資源寿命を扱うWP-4へ進む。既定v3は維持。

## WL-024 — Step 10: WP-3 fix4再受入（2026-09-08）

- durationScale製品配線、共通raw duration、公式raw/Scale別最終frame、fixture-case表示を独立確認。CPU/CUDAと実WAVもpass。
- REQ-05のWP-3技術部分はpass。REQ-04は製品`buildDurationFeatures`全14値と公式fixtureを比較するE1接続だけが残る。
- 詳細は [fix4独立受入](evidence/wp3-acceptance-round5.md)。小さいfresh fix workerへ残る一点を委譲する。

## WL-025 — Step 10: WP-3 E1 duration feature parity fix5（2026-09-08）

- 対象: WP-3の最後のE1一点のみ。fixtureの `duration_feature_text`、token count、max text length、speaker/caption flagsをGo parityが読み、製品 `buildDurationFeatures` を実行して公式 `duration_features` の全14要素を `atol=1e-4`/`rtol=1e-3` で比較するよう修正した。公式期待ベクトルをNN入力へ直接渡す経路は使用しない。入力flagsもfixture metadataと一致検証する。
- fixture generatorを更新して条件ごとの独立14要素期待値と入力metadataを保存し、公式固定資産から再生成した。fixture SHA256は `1aa903e3e9c3e65a14de4f2ca0896b03caf289fbf3bcdaff92dbdcd150713c72`、manifestの `fixtures.sha256` と一致する。
- テスト: targeted `go test ./internal/localtts/irodori/pipeline ./cmd/tts-parity` pass（4条件の14要素比較、1要素改竄拒否を含む）。expanded direct-consumer suite pass。製品v4 pipelineのCPU/CUDA parityは両方 pass（6 graph、4条件、CFG=1/default、40 step、atol/rtol維持）。
- 証拠: [WP-3 fix5 worker](evidence/wp3-fix5-worker.json)。Acceptance/state欄およびWP-4/5は変更していない。

## WL-026 — Step 10: WP-4サービス統合・移行・取消実装（2026-09-08）

- 実装: v4 manifestのschema/graph契約に加えてtokenizer、全graph model、external dataのSHA256をTalk開始前に検査。未知schema、欠損、サイズ不一致、サイズを保った破損は説明付きで拒否し、v3 metadata.json経路と既定modelDirは維持した。
- サービス: 1回の複数文TalkごとにRuntimeを1つだけロードし、文ごとにoptions/outputだけ差し替えて再利用。文間300ms、失敗文3秒無音を維持し、全文章失敗/全無音は成功扱いしない。取消時はORT推論goroutineの終了をjoinしてからRuntime.Closeし、一時WAVを削除する。
- 設定/task: modelDir/narratorDir/refWavの明示保存・再読込とv3既定をテスト。`cmd/tts-smoke` / `tts-smoke` taskにmodel、EP、seed、ref、seconds、cancel、`--expect-error`を追加し、後続のCPU/CUDA/auto別プロセス検証条件を固定した。
- 検証: WP-4 targeted Go test（localtts/store/generation/tts-smoke）pass、frontend build pass。全Go回帰は表示された全パッケージpass、残りmusicgen/player/rss/talkも独立pass。`tts-smoke --service` をCPU/seed0/steps2/seconds0.5/既存narratorで実行し、複数文の124844-byte・48kHz mono WAV（約1.3秒、文間300ms相当）を生成した。EP別実機、BGM共存、取消後再生成、Wails操作、WP-5性能/品質/E2Eは未実施。証拠: [WP-4 worker](evidence/wp4-worker.json)。
- 状態: REQ-06実装状態をimplemented、REQ-05/07/08をin_progressへ更新したが、Acceptance欄とplan stateは変更していない。既定切替は行っていない。

## WL-027 — Step 10: WP-4 fix1 共有Runtime終了・公開API回帰（2026-09-08）

- 修正: v4 `Runtime` が `LoadInitialise` の options を保持し、公開 `Synthesize()` と zero `SynthesizeWithOptions` が text/output を消さないようにした。Runtime は実行中推論を待ってから Close し、ロード/Close 回数の観測点を追加した。opt-in 実v4 CPU ORT 回帰（初回→更新→zero options）に成功。
- 修正: direct/service `tts-smoke` を `run() error` 構造へ変更し、取消後の成功WAV採用を禁止、推論join→Close→一時/最終WAV削除を出力で識別可能にした。実CPU in-flight取消は `started=true, joined=true, closed=true, load_count=1, close_count=1`、既存出力なしで終了した。
- 修正: Player の所有context/WaitGroup、app shutdown の active Talk/BGM/prefetch join、musicgen の取消join・出力削除・Runtime Close guardを実装。同一Serviceの実取消→後続生成とTalk単位load count=1を確認した。
- 追加: shutdown/cancel競合時に同期Talk/BGM・prefetchの成功結果やfallbackを採用せず、Talk一時WAVもctx取消後に削除する境界チェックを追加した。
- 検証: targeted/full `go test`、CPU `tts-parity`、frontend build、`mise run build`、公開Synthesize回帰、同一Service再生成、実CPU service/direct smoke（取消を含む）をpass。CUDA/auto、BGM/Talk連続E2E、Wails手動、WP-5は未検証。Acceptance欄とplan stateは変更していない。詳細は [WP-4 fix1 worker](evidence/wp4-fix1-worker.json)。

## WL-028 — Step 10: WP-4 fix2 publish/終了同期と実イベント観測（2026-09-08）

- Player: Talk/BGM/prefetchの結果公開をctx・closed・generationの同一mutex境界へ統合。Skip/UpdateConfig/Shutdownで世代を進め、旧workerのready/error復活を抑止。決定的publication gateテストを追加。
- Runtime: Irodori/Stable Audioの並行CloseをcloseDoneで同一完了まで待機させ、推論中Closeのjoin順序を固定。並行Closeテストを追加。
- 観測: Irodori runtime/serviceへload、inference start/end、join、close start/endの実イベントを追加。tts-smokeは実start signalでcancelし、join→Close、temp/final出力なし、同一Service/process後続生成を検証。
- 検証: 対象Go test、race、`go test ./...`、frontend build、`mise run build`、CPU/CUDA `tts-parity`、実CPU service/direct成功・取消smoke、auto service smokeがpass。詳細は [WP-4 fix2 worker](evidence/wp4-fix2-worker.json)。Wails手動BGM/Talk連続E2E、WP-5は未検証。Acceptance欄とplan stateは変更していない。

## WL-029 — Step 10: WP-4 fix3 検証不変条件とClose barrier（2026-09-08）

- `cmd/tts-smoke` のエラー種別を operational / inference / cancellation / invariant に分離し、`--expect-error` は要求した推論・取消エラーだけを期待成功として扱うよう修正。入力不正（`;` 単独の正規化空文字）、preflight/load、lifecycle順序・イベント不足、temp/output残存、load-close count不一致、取消後再生成失敗は常に非zeroとした。
- Irodori/Stable AudioのCloseテストに実Close開始後かつin-flight待機直前の `closeWaitHook` barrierを追加。先行Closeの待機を観測してから第2Closeを開始し、双方が同一closeDone完了まで戻らないことを決定的に検証する。
- 検証: targeted Go test、`go test ./...`、`go test -race ./...`、frontend build、`mise run build`、CPU `tts-parity` がpass。Venueケースとして `--text ';' --cancel-after 1s --expect-error` と不在modelのpreflight `--expect-error` は非zero、正常service cancel+expect-errorは同一Service再生成・temp/output cleanup・load_count=2/close_count=2を確認、direct cancelはload_count=1/close_count=1を確認した。詳細は [WP-4 fix3 worker](evidence/wp4-fix3-worker.json)。Acceptance欄・plan state・WP-5は変更していない。

## WL-031 — Step 10: WP-5初回独立受入（2026-09-08）

- full/race/frontend/Wails build、CPU parity、限定local E2Eはpass。10原稿は200〜235字、短条件60 WAVは全seed/モデル/hash/形式を確認。
- benchmarkの製品Talk迂回、VRAM unavailableの0扱い、E2EのPlayer/BGM未実行と固定true、正解読み不一致、正式条件/証拠metadata不足を検出。
- 判定: WP-5 fail。REQ-08〜10をin_progressへ戻す。詳細は [WP-5初回独立受入](evidence/wp5-acceptance-round1.md)。fresh fix workerへ検証器修正を委譲する。

## WL-032 — Step 10: WP-5 fix1独立受入（2026-09-08）

- 製品Service Talk単位、実Stable Audio/Player、VRAM polling、exact seedは確認。full/race/Wails buildもpass。
- 時間イベント境界、p95/期限/VRAM gate、WP-4 smoke回帰、完全3周期/active shutdown/E2E失敗集約、読み表/provenanceに残件。
- 判定: 正式40step全量へ進行不可。詳細は [WP-5 fix1独立受入](evidence/wp5-acceptance-round2.md)。fresh fix workerへ委譲する。

## WL-033 — Step 10: WP-5 fix2独立受入（2026-09-08）

- 時間境界、nearest-rank p95、deadline/VRAM fail-close、smoke回帰、3完全Talk→BGM周期、設定child、異常集約を独立確認。
- 残件はactive inference同期shutdown、report provenance、読み/仕様誤記、計画外range制約、VRAM測定手段。正式40stepは未実施。
- 詳細は [WP-5 fix2独立受入](evidence/wp5-acceptance-round3.md)。限定fresh fix workerへ委譲する。

## WL-030 — Step 10: WP-4最終独立受入（2026-09-08）

- fix3のCLI分類を独立負例で確認。正常cancel→join→Close→同一Service再生成、load/close count、一時/最終WAVなしもpass。
- 前回のfull regression、CPU/CUDA parity、CUDA/auto複数文を保持。publish gate、closeDone、app shutdown順序をE3確認。
- 判定: WP-4製品実装pass。REQ-05 pass、REQ-06/07のWP-4部分pass。実設定再起動、BGM/Talk統合、性能はWP-5へ。詳細は [WP-4最終独立受入](evidence/wp4-acceptance-final.md)。

## WL-034 — Step 10: WP-5 fix3検証器修正（2026-09-08）

- active shutdown: Talk、Stable Audio BGM、Talk+BGM prefetchの3ケースで予約flagではなく実推論開始イベントを待つ短E2Eを追加。前版で3/3実行結果を確認後、複合ケースの同時activeを保証するobserver barrier（全対象の実start観測後に解除→Shutdown）へ強化した。強化版の再実行はCPU時間超過で停止指示により中断し、既存reportは強化版の証拠として扱わない。結果と制限は [WP-5 fix3 worker](evidence/wp5-fix3-worker.json) に記録。
- benchmark provenance: jjの40桁snapshot/parent、steps/seconds/CFG/scale、model/reference/input、metadata/tokenizer/graph、ORT/EP/GPU/driverを各reportへ統一保存する経路を修正。v3 childが失敗してもv4 child・compare・集約reportまで実行する。
- benchmark gate: last5 max-minは診断値に限定し、合否はlast5各値と最大値がfirst5定常中央値+512MiB以内とした。Windows WDDMでprocess used_memoryがN/Aの場合は同じnvidia-smiのdevice `memory.used`を100ms pollingし、source/index/UUID/baselineと他process混入リスクを記録、取得不能はfail-closeする。
- expected reading/docs: 10原稿の原文対応を点検し、script04の「三日前」を「みっかまえ」へ修正。製品bundleではfixture hashを必須にせず、parity時のみ検証する仕様へ修正した。
- 資産整理: `evidence/tts-benchmark-steps2` の既知worker生成tracked WAV20件のみ削除し、JSON/CSV診断は保持した。既定v4、Acceptance/state、正式40step全量は変更・実行していない。
- 検証: `go test ./... -count=1`、対象race、対象vet、旧short CPU E2E（3/3）、最終barrier変更後の `go test ./cmd/tts-e2e ./cmd/tts-benchmark -count=1` がpass。正式40step性能測定、short v3/v4 benchmark、Wails手動確認は未実施。最終barrier版E2EはCPU時間超過で中断。

## WL-035 — Step 10: WP-5 fix3独立受入（2026-09-08）

- 最終barrier版を新規CUDA短条件で実行し、Talk→BGM 3周期、Talk/BGM/複合active shutdown、設定child、異常系、終了後の状態・WAV残留なしを確認。全Go test、対象race、全vetもpass。
- 短benchmarkでv3失敗後のv4実行、両report集約、deadline/+512 gateの負例拒否を確認。WDDMでは全件device-totalであり、process VRAM/リーク判定には使用しない。
- 残件: Player旧世代workerが新世代prefetch flagを解除しうる競合、combined reportのsteps/seconds固定値とprovenance不足、取消受理時点までの実推論・イベント順序・Service joinの検証不足。
- 判定: 正式40step全量へ進行不可。詳細は [WP-5 fix3独立受入](evidence/wp5-acceptance-round4.md)。限定fresh fix workerへ委譲する。既定v3を維持。
## WL-031 — Step 9: WP-5 benchmark/E2E実装（2026-09-08）

- 実装: `cmd/tts-benchmark`（固定10原稿、各文/全原稿、cold load/RTF/期限、CSV/JSON、20定常反復、v3/v4別process、REQ-09の60 WAV manifestモード）、`cmd/tts-e2e`（local RSS/LLM fixture、実v4.1、audio/loudness、BGM復帰、隔離config再起動相当、preflight負例、取消join/close）、`store.NewAt`、mise task、現行README/requirement/specification/cheatsheetを追加した。既定v3とAcceptance欄は変更していない。
- 実行: `mise run tts-e2e` CPU 3/3周期成功。`mise x -- go test ./... -count=1`、frontend build、`mise run build`成功。CUDA短条件benchmarkはv3/v4別process各85行・失敗0、cold load（v3 2085.68ms、v4.1 6874.067ms）、v4定常20回 first5中央値200.35ms/last5中央値180.92msを保存。短条件の全原稿p95はv3 408.30ms/v4.1 520.43ms（比1.27）であり、40-step基準の代用にはしない。nvidia-smi process PIDは当該子process値を取得できずVRAM判定は未完了。
- REQ-09補助: steps=2の60 WAV（v3/v4.1×seed0/1/2）とhash/text/expected_reading manifestを生成した。40-step試聴はv3 seed0 script02までで時間超過のため未完了。読み・反復・末尾切れ・数字/固有名詞・声質・自然さは人間判定待ちであり、自動passにしていない。

## WL-033 — Step 10: WP-5 fix2 検証器境界・gate修正（2026-09-08）

- localtts observerを真のpreflight/runtime-load/sentence/gap/combine/close境界へ分離し、benchmarkでtrace順序・非重複内訳を検証。nearest-rank p95（`ceil(.95*n)-1`）、single/compare双方の10 Talk・20 steady deadline、VRAM全件有効、first5/last5の+512/rangeをgate化し、欠測/未達はnonzeroとした。
- tts-smokeは新lifecycle順、preflight/loadのexpect-error誤成功防止、所有一時WAVの再帰監視/cleanupへ対応。normal/cancel/missing model負例を短条件で確認した。
- tts-e2eはTalk完了→後続BGM復帰の完全cycleを3回数え、実Player prefetch active shutdownと`Player.Shutdown`→`generation.Shutdown`を観測。short CPUで`cycles_ok=3/3, errors=0`、schema999正確error、skip/regenerateを確認した。
- report provenance（jj snapshot/parent、条件、hash、ORT/EP/GPU/driver、v3 metadata/tokenizer/graph）とignored formal WAV procedureを追加。正式40step/RTX 5090性能・60 WAV試聴は未実施。詳細: [WP-5 fix2 worker](evidence/wp5-fix2-worker.json)。

## WL-036 — Step 9: WP-5残件修正の再開（2026-09-08）

- leader: 既存snapshot `a7cc343b014335c127e9eca4367a08200c17bed5` とWP-5 fix3独立受入を起点に、ユーザー依頼で残件修正・実装確認を再開した。既存未commit変更を保持する。
- fresh workerへPlayer世代競合、計測の実条件/provenance、取消受理/推論終了/join/closeの同期証明を委譲。世代所有権と実イベントbarrierを助言した。影響対象と直接consumerのexpanded検証を選び、正式全量実行は検証器の再受入後に進める。
- persistent acceptanceへ旧証拠の追跡可能性・有効性、未検証の必須checkと実行環境の監査を依頼。stateはimplementing。人間聴取と実Wails確認は自動検証と区別し、既定v3を維持する。

## WL-037 — Step 10: 旧証拠と正式計測条件の独立監査（2026-09-08）

- acceptanceが固定資産hash・数値core差分を確認し、REQ-01〜04の保持可能範囲とREQ-05〜10の残検証を整理した。[初期監査](evidence/acceptance-evidence-audit-20260908.md)。不足する必須回帰の独立実行は修正統合後に補完する。
- 正式Talk期限は60,000ms。独立コード調査で既定BGM30秒×3曲の2曲目返却前に先読み開始し、残り2曲×30秒が予算となることを確認した。gap/生成待ちは加算しない保守値であり、Talk音声の目標長とは区別する。
- WDDMのdevice-totalをprocess VRAMに代用しない。Windows GPUProcessMemoryのPID別DedicatedUsage取得可能性が判明し、実TTS PID/adapter対応を独立調査中。正式性能passには有効な測定証拠が必要。

## WL-038 — Step 9/10: WP-5 fix4報告と追加計測修正（2026-09-08）

- fresh workerがPlayer世代＋所有者tokenによるcleanup制御、取消時のactive/event/barrier検証、combined条件/provenance修正を報告した。[worker報告](evidence/wp5-fix4-worker.md)。targeted Go/race/vetはworker passであり、独立受入は別途行う。
- acceptanceへPlayer/Service/取消境界のexpanded独立再検証を依頼。benchmarkはWindows PID別VRAM collector追加と親子reportのsnapshot自己干渉修正が残るため、別fresh workerへ担当範囲を区切って委譲した。
- 正式全量は検証器の短parent/child確認後に実施する。正式出力はignored build/bin/acceptance配下へ隔離し、計測中のtracked編集を凍結する。stateはimplementingを維持する。

## WL-039 — Step 10: Player/取消fix4独立受入（2026-09-08）

- [round5](evidence/wp5-acceptance-round5.md)が修正範囲passを支持。関連Go/race成功、固定binary短CUDA E2E158秒・3周期成功・errors0。3 shutdownケースのstart→accepted→end→runtime join→ServiceJoin→CloseStart→CloseEndを確認した。
- leaderは当該限定passを採用。REQ-07/10の残りCPU/auto・正式条件・WebViewを通過扱いしない。persistent acceptanceへCPU/auto fallback等の未検証E1続行を依頼。
- 数値core/fixture/ORT条件不変によりREQ-03/04の既存parityを保持する。benchmark修正は継続中でstateはimplementing。

## WL-040 — Step 10: EP/取消追加独立受入（2026-09-08）

- [round6](evidence/wp5-acceptance-round6.md): CUDA強制失敗（説明付きexit1/WAVなし）、CPU短3周期396秒、強制CPU fallbackのauto短3周期407秒、同Service取消→再生成46秒を独立確認した。
- 設定childのv4/v3復帰、Skip後再生成、active shutdown順序、専用TEMP空、load2/close2を確認。round5と合わせREQ-07のEP/取消/共有BGM/Talk自動必須範囲passをleaderが採用する。
- REQ-06の実Wails Settings、REQ-10正式全文/WebView、REQ-08/09正式計測/品質は未検証を維持。計測器修正中でstateはimplementing。

## WL-041 — Step 10/9: GPU collector独立負例と追加修正（2026-09-08）

- Windows process collectorのworker実装報告後、[round7独立受入](evidence/wp5-acceptance-round7.md)で3件failを確認: 途中error/EOF後も古い値をavailable扱い、WQL PID部分一致による他PID混入、文peakがTalk累積で最終Stop観測も破棄される。
- leaderはfailを採用しfresh fix workerへ委譲。timestamp/sequenceによる区間とTalk全体の分離、PID/LUIDの厳密照合、測定完全性のfail-closeを助言した。既存unit/raceのpassを負例の代用にしない。
- 高価な正式実行は修正再受入後へ。round5/6のPlayer/取消/EP証拠は無関係のため保持。REQ-08はin_progress、state implementing。

## WL-042 — Step 10/9: provenance境界の独立受入と限定修正（2026-09-08）

- [round8](evidence/wp5-acceptance-round8.md)で任意out subtreeによる製品hash除外と架空provenance採用をfail、短parent診断のCLI拒否を未検証要因として確認。親固定・両子継承の既存修正は部分確認したが正常combinedは未受入。
- fresh workerへ出力除外境界、継承identity実在/parent対応、実行中parent変更検出、scripts1/steady1診断の対応を委譲。診断を許しても正式10原稿/20反復のgateは維持する。
- 先行workerのCPU全30Talk診断は過大だったためleaderがcheckpointで結果を回収し、今後は最小両child診断へ制限。GPU collector修正と統合後に独立E1を実行する。state implementing、既定v3、round5/6保持。

## WL-043 — Step 10/9: 計測完全性の再受入と正式E2E開始（2026-09-08）

- round9で欠測後fresh sampleによる復帰、任意wait error許容、内部時刻のmonotonic欠落を確認しfresh GPU fix2へ一括委譲した。worker報告は[GPU fix2](evidence/wp5-gpu-fix2-worker.md)、独立再受入で負例全体を再確認する。
- round10で継承hashと実tree/snapshotの未照合を確認しfresh provenance fix2へ委譲。各child開始/終了の実parent/hash、captured snapshot diffhashの一致、既存build/bin/別drive外部出力対応に限定する。
- benchmark修正とは独立して、round5/6と同じE2E binary/sourcehashで正式CUDA40step・自動duration・3周期を開始した。GPU workloadは逐次とし、計測器実PID観測を重ねない。完了前に正式E2Eをpass扱いしない。

## WL-044 — Step 10: 正式CUDA E2E独立pass（2026-09-08）

- [round12](evidence/wp5-acceptance-round12.md)によりCUDA40step/自動duration/3周期を188秒、errors0で独立確認。設定v4保存/child再起動/v3復帰、skip/regenerate、HTTP/loudness、preflight異常、3取消caseの順序/idleを確認した。leaderはREQ-10正式製品E2E部分passを採用。
- BGM1step/1秒はE2E専用条件でありREQ-08の60秒期限根拠や性能比較へ外挿しない。実WebView/Settings/人間品質は別途残す。
- GPU collectorはWindows二重Killによる正常Stop誤失敗が残りfresh限定修正中。provenance identity fix2は実装報告済みで独立再受入中。state implementingを維持する。

## WL-045 — Step 10/11: 統合検証器pass・最終受入へ移行（2026-09-08）

- [round13](evidence/wp5-acceptance-round13.md): 全Go/race、実Windows Stop、実TTS PID/WDDM外部照合、provenance実jj負例一括、短両child/combinedを独立確認。短parent40.5秒のnonzeroは1/1件数不足等による期待gateであり正式性能を意味しない。
- 既知実装修正なし、全製品/検証器を統合しleaderがacceptingへ移行。implementedと正式性能/品質passを区別してstatus/index同期。cheatsheetに確認済みPID測定/欠測/所有Stop/provenance境界を反映した。
- persistent acceptanceへ最終受入を依頼する。保持証拠は再実行せず、正式CUDA40step性能10+20→正式60 WAVを逐次実行。長run中tracked編集を凍結しbuild/bin/acceptanceへ保存、終了後に証拠をコピーする。
- 実Wails表示/Settings保持は隔離launcherで人間確認を依頼済み。生成操作はGPU計測後へ、人間E4は60 WAV完成後へ。done/既定v4切替は全ゲートまで行わない。

## WL-046 — Step 10/11: 正式受入結果と人間引渡し（2026-09-08）

- [round14](evidence/wp5-acceptance-round14.md)で正式benchmark981秒/exit1と正式audition976秒/exit0を独立評価。生成失敗0、両170CSV行・全音声hash/条件一致。REQ-08はp95比1.93365とv4 VRAM上限+1064MiBでfail、期限60秒は全件pass。Talk05の991ms sample gap欠測は別途保持。
- 正式60 WAV/30ペア（計42.3分）を独立形式検証し、listen.html/正解読み/未確認評価欄をユーザーへ引き渡した。人間品質は未確認。ユーザーの実Wails起動・v4.1/v3保存→終了→再起動保持の回答を独立担当がREQ-06 passと評価した。
- leaderはstatus/index/README/現行requirement/specification/cheatsheetへ事実を反映。古いdevice-total説明とREQ-07の正式E2E待ち記述を訂正した。REQ-01〜07 pass、REQ-08 fail、REQ-09/10 human-pending、state accepting、既定v3を維持。

## WL-047 — Step 10: 性能未達の限定診断と方針確認（2026-09-08）

- fresh read-only workerが正式内訳を分析し、差の66%がpreflight/load、20steadyの入力/seed等同一、VRAM値は変動しリーク断定不可と整理した。[診断](evidence/performance-followup.md)。
- fresh verification-only workerが正式生成後にignored領域のdirect pipeline probeを1回実行。同Runtime再利用でload+Talk約21.2秒→次Talk約14.0秒だが、Service hash/結合/WAV検証/BGM共存を含まないため正式pass代用不可。製品常駐cacheは未採用。
- leaderは現仕様の1 Talk内再利用を広げる判断として、v3既定で受入保留か、BGM共存を含む常駐再利用検証・最適化継続かユーザーへ確認を依頼。全GPU診断終了を通知し、実Wails音声操作と生成中終了の人間確認も依頼した。回答前に仕様/基準を変更しない。

## WL-048 — Step 11: 最終文書整合性（2026-09-08）

- 独立delta確認で86ローカルリンク解決、診断4コピーhash一致、正式以降の製品/依存/config差分なし、既定v3一致を確認した。
- 残る現行specの旧device-total文をprocess DedicatedUsageへ訂正。ignored診断Go sourceも.go.txtへ退避し通常package探索から外した。再現手順と証拠は保持し、製品コードは変更しない。
- REQ10文書部分のdelta再確認を独立担当へ依頼。REQ08未達/REQ09と実音声操作の人間待ち/最適化方針回答待ちは変わらない。

## WL-049 — Step 11: 最終文書pass記録（2026-09-08）

- [独立最終文書確認](evidence/acceptance-final-docs.md)がREQ-10 docs部分passを確定。対象snapshot23aef6e3839b17d4dd8c3ce9032366ed5f17d20f、製品差分なし。statusの文書待ちをpassへ反映し、この記録のみ追加。独立担当が無害状態記録として再検証不要と判断した。
- 最終残件はREQ-08性能未達と最適化方針、REQ-09品質E4、REQ-10実音声操作。state accepting、既定v3、done未設定を維持。

## WL-050 — Step 9/10: 利用者採用判断を受けた基準改訂（2026-09-08）

- 利用者が両版の聞き取り品質を許容し、v3同等資源の追求は不要としてv4.1を明示承認。REQ08の比較数値/VRAMを診断、REQ09を総合品質許容E4へ改訂し、証拠原文と旧結果を保持した。
- leaderは新規既定v4.1とbenchmark採用gateを限定実装するためaccepting→implementing、REQ06/08/10の差分をin_progress/unverifiedへ戻した。既存保存値は保持、常駐cache/機能UI追加なし。
- 同一acceptanceへ改訂判断と旧証拠保持を依頼。性能/60 WAV再生成は不要、既定・判定・表示consumerのtargeted/expanded検証を採用する。実UI未観測は採用と分離し未確認を維持。

## WL-051 — Step 9/10: v4.1採用差分の実装完了（2026-09-08）

- fresh workerが新規store既定とUI placeholderをv4.1へ変更し、benchmarkのadoption/comparison_diagnosticsを分離した。[実装証拠](evidence/wp5-adoption-worker.md)。旧比較値/欠測を維持し、保存済みv3/任意path/ref/narrator保持、正式件数/生成/期限/条件を検証した。
- workerのstore/benchmark testsとfrontend buildはpass。leaderは既知実装残件なしとしてacceptingへ戻し、同一acceptanceへ最小独立checksとWails buildを依頼する。長時間の数値/性能/品質再生成は不要。

## WL-052 — Step 10/9: 採用レポート境界の限定補正（2026-09-08）

- 独立担当がadoption表示とsentence/child失敗の対応、およびadoption追加writeが最終provenance確認の後になる問題を検出。判定の実exit対応とwrite→最終検査の既存契約を保つためfresh workerへcmd限定修正を委譲する。
- leaderはaccepting→implementing、REQ08差分のみin_progress/unverifiedへ戻す。新規既定・保存設定の検査とWails buildは独立に続行し、推論/性能の全量反復は行わない。

## WL-053 採用report補正の再受入
- state: implementing → accepting。sentence生成失敗/child failure集約と最終write後provenance guardを補正。REQ-08実装済み、差分受入はunverified。証拠: evidence/wp5-adoption-fix1-worker.md。同一acceptance担当へ限定再検証を依頼。

## WL-054 v4.1採用後の独立受入完了
- 証拠: evidence/wp5-adoption-acceptance.md。REQ-01～09はpassまたは有効証拠保持。新規既定v4.1・設定互換・採用report整合は独立pass。正式測定/60 WAVを再生成せず有効証拠を保持。
- current docs/README/cheatsheetも独立delta pass。REQ-10は自動E2E・回帰・新Wails/frontend build・文書pass、実UI音声操作のみhuman-pending。モデル採用は確定し、計画stateはaccepting。
- leaderは本報告に基づき受入表/index/履歴だけを更新。追加の製品変更・テスト再実行は不要。

## WL-055 最終利用者承認と計画完了（2026-09-08）
- 利用者の起動・一通りの動作確認/問題なし/現状承認をevidence/user-acceptance-final.mdへ原文保存。REQ-10の総合受入E4として独立評価。個別操作ログ/実行EXE hashは補完しない。
- evidence/acceptance-completion.mdで全完了gate充足。対象snapshot 21cc197b46b87b936d9daf9a1a11c875025ce760、製品差分なし、current docsのdelta pass。必須自動検証・モデル品質・既存設定保持の有効証拠を保持し、再実行なし。
- leaderがREQ-10をpass、stateをaccepting → doneへ更新しindexを同期。全REQ/WP完了、未完了項目なし。変更は文書/受入記録のみ。

## WL-056 検証成果物の追跡整理
- 利用者指示により /evidence/ と /docs/**/evidence/ をignoreし、jj file untrackで317ファイルを管理対象から除外。ローカル317ファイルの存在とSHA256不変を確認。親リビジョンで追跡済みの69件は削除/移動差分になる。過去リビジョンは書き換えない。
- ライセンス2件をdocs/licenses/irodoriへ分離し、exporterのnotice参照を更新。原文hash一致を確認。受入要約を92_acceptance_summary.mdに保存し、詳細evidence参照はローカル履歴と明記。製品アプリコード/モデルは変更なし。
