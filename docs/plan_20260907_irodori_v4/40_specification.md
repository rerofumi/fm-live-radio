# 実装仕様と段階的な移行手順

対象: v4.1 第一候補 / Go + ONNX 維持。採用は **条件付き**。対応 exporter と ONNX graph の成功証拠はまだなく、WP-1 の Go 判定前に既定切替しない。

## 現行コードから変わる点

| 対象 | 現在 | 必要な変更 | REQ |
| --- | --- | --- | --- |
| tokenizer/tokenizer.go | Unigram独自実装、先頭metaspace付与、PAD候補が旧名 | v4 tokenizer JSONのpre_tokenizer/特殊tokenを解釈し、公式fixturesで検証。v3経路を保持 | 02 |
| metadata/metadata.go | v2/v3の既定値を補完、version無し | schema_version/model_family/source revisions、tokenizer情報、speaker patch、graph入出力/shape、external data一覧を明示 | 01/03/06 |
| pipeline/pipeline.go | IsCaptionMode による排他分岐。caption時はdurationを使わず10秒へfallback | caption/speaker独立、pretrained text/caption encoder、patch後の長さ、dual duration、null条件、CFGを対応 | 03/04/05 |
| sampler/sampler.go | 現行v3のCFG分岐 | 公式v4の条件drop/mask、CFG適用時刻と式に整合。既存v3の挙動を維持 | 04 |
| localtts/service.go | 文ごとにload/close。推論goroutineの取消時にdefer Close | v4事前検査を文失敗処理の外で実施。必要な再利用と実行中資源の寿命を設計 | 05/06/07/08 |
| store/store.go、frontend/src/App.tsx | v3既定ディレクトリ、v3 placeholder | 試験時は明示modelDir。全ゲート通過後のみ新規設定既定値と説明を更新 | 06/10 |
| README.md、docs/requirement.md、docs/specification.md | v3の利用・変換手順 | 実装完了後にas-builtへ更新、存在しない旧cmd/local_smoketest案内を置換 | 10 |

パスの基点は `internal/localtts/irodori/`（表内で省略したもの）。domain.IrodoriConfig に caption UI 用設定は今回追加しない。

## WP-1: v4.1 ONNX export の実現性と固定成果物

REQ-01/03。新規の再現可能な exporter プロジェクトを `tools/irodori_export/` に置く案とする。開発専用の Python/uv/mise/lock を固定し、配布アプリに Python を要求しない。既存 mtsmfm exporter は fork の古い推論 revision に固定され、実測でも wrapper が失敗するため、そのまま依存先を差し替えない。

1. 今回の固定公式ソース・model/tokenizer/codecを入力として使用する。ModernBERT共有backbone+projectorを含むtext/caption encoderを最小exportする。
2. CPU ORTで公式と同じids/maskを入力しparityを確認。その後CUDAを確認。失敗時は演算/shape/precision別に原因を記録する。
3. speaker encoder、dual duration、両条件のDiT step、DACVAEをexportし、全graphで動的長とparityを確認する。caption省略時もspeaker経路が残ることを確認する。
4. 成果物は `model/irodori-v4.1/` に分離。text/caption共有重みの重複、外部weight、サイズ、メモリを計測する。opsetは現行18を初期候補とし、成功した値をmanifestに固定する。
5. graph全体のparityと短文ORT生成が成功すればGo。失敗時は既定v3を維持してWP-1をblockedにし、原因と代替案をレビューする。公式Pythonの成功をONNX成功の代用にしない。

manifest案: schema_version=2、model_family=irodori-v4、model_release=v4.1-small、source各revision、precision、opset、sample_rate、hop_length、text/caption最大長、speaker_patch_size、max_ref_seconds、tokenizerファイルとspecial IDs、全graphの入出力名/型/shape、external dataとhash。未知schemaは拒否する。schema無しの既存v3はlegacy loaderで読む。

## WP-2: tokenizer parity

REQ-02。公式の `PretrainedTextTokenizer` を正解系とし、モデルに同梱された tokenizer を使う。今回の6ケースの差を直し、追加ケースを fixtures にする。現在の v4 pad=3 と v3専用fallback=4、metaspace prepend_schemeの差は確認済み。Unicode/byte fallback、空白保持、特殊token、BOS、256token境界を含め完全一致を要求する。パーサが読めるだけでは完了しない。

基本案は既存Go実装のバージョン別拡張。汎用tokenizer binding導入は未調査で未採用。完全一致を安定して実現できない場合に別案として調査する。

## WP-3: Go推論実装

REQ-03/04/05。WP-1で確定したmanifestを契約にして実装する。

- v3をlegacy分岐で維持する。v4はcaption/speakerの有無を独立管理し、名前から明示的にtensorを作る。順序が一致しているという推測に依存しない。
- text/captionはModernBERT側の正規化・mask・最大長に合わせる。アプリcaption空でも公式と同じnull条件を構築する。
- speaker patch=4、末尾端数、null token、mask出力長をmanifestと実graphから計算する。現行の `speakerLen=refLen` を流用しない。
- durationはtext/speaker/captionを含む公式条件から計算し、seconds指定、durationScale、0.5–30秒の制限を整合させる。120秒は参照上限であり、生成上限へ転用しない。
- 公式にある参照音量正規化、resample、tail trim、text normalization、CFG時刻範囲（今回Python既定は0.5–1、現行Goは0–1）を比較し、v4向けの仕様をfixturesで固定する。
- SilentCipherは公式Python実測で有効だった。既存Go経路には無い。今回のGo移行に透かし実装は追加せず、その差を明示し、ONNX parityは透かし適用前tensorで比較する。透かし対応を必要とする配布方針に変わる場合は追加要件化する。

## WP-4: サービス統合・移行・取消

REQ-05/06/07/08。対応manifestの事前検査でモデル一式・external data・tokenizerのSHA256を検査する。存在/サイズだけでは破損を検出できない（今回実測）。検査時間はロード時間へ計上し、構造エラーを3秒無音へ隠さない。既存のWAV選択、文分割、文間、失敗文ポリシーを維持する。

取消時の現在のコードは `ctx.Done()` で戻る一方、推論goroutineと `defer rt.Close()` が並走し得る（静的観察、クラッシュ実測なし）。v4対応時はin-flight実行終了を待ってから破棄する所有者を設ける。Goバインディングの実行中断APIは未調査なので採用前提にしない。取消後の一時WAV削除と後続リクエストを検証する。

まず再利用範囲を1回の複数文Talk内に限定し、モデル/参照条件の変更で作り直す。文ごと再ロードのコストを避ける。常駐キャッシュはBGMとのGPU競合を測ってから検討する。

設定移行: 保存済みmodelDir/RefWAV/narratorDirを保持。v4.1一式を別配置し、Settingsで明示指定して再起動・生成を試験する。受入後のみ新規設定の既定値を変更。失敗時は既存v3ディレクトリへ戻して再起動し、同じ短文の生成・再生で復帰を確認する。

## WP-5: 受入と文書反映

REQ-07–10。上流品質比較、Go/ORT parity、アプリ品質・性能、core E2Eを分離する。受入は実装とは別セッション/人が行い、90_status.mdのacceptanceを記入する。

### Core E2E

1. 旧v3設定で起動→設定保持→v3 Talk生成。v4.1の明示指定→保存→再起動→モデル識別と単一参照で生成。
2. 固定RSS/LLMレスポンスをローカルfixtureで供給→200–300字の複数文原稿→実v4.1 ONNX→48 kHz WAV→audio server→Talk再生→BGM復帰。ネットワーク上のニュース変化から分離する。
3. 文間300msと失敗文3秒無音を確認。全graph欠損や非互換は事前エラーであり、正常Talkとは判定しない。
4. 停止/スキップ/再生成、取消中の終了、参照WAV欠損、external data欠損、サイズを保ったweight破損、未知schema、空原稿を確認する。
5. BGM生成とTalk先読みを少なくとも3周期継続。CUDA/auto fallback/CPUは共有ORTが一度しか初期化されないため別プロセスで検証する。v3への復帰も再起動して確認する。
6. 実Wails WebViewの再生、音量、停止、BGM復帰を手動で確認する。ブラウザーのみの試験をデスクトップE2E完了としない。

### コマンドと完了条件

既存コマンド: `mise install`、`mise x -- go test ./...`、`mise x -- npm --prefix frontend run build`、`mise run build`。未導入のfrontend依存は `mise x -- npm --prefix frontend ci`、Wailsが無ければ既存 `mise run setup` を使う。今回の実行はTTS周辺のgo testのみ。

今後追加するtask（現在は存在しない）: `mise run tts-export-v4`（REQ-01/03）、`mise run tts-parity`（REQ-02/03/04）、`mise run tts-smoke`（REQ-05/06/07）、`mise run tts-benchmark`（REQ-08）、`mise run tts-e2e`（REQ-10）。taskは将来WPでmise.tomlに登録し、入力model/EP/seedを明示可能にする。品質聴取結果はREQ-09の別表に残す。

全REQ合格後にREADME/現行requirement/specification/cheatsheetを更新する。今回作成した計画をそのまま現行仕様にコピーしない。

