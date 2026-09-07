# 移行計画ステータス

受入結果の共有用記録は[受入要約](92_acceptance_summary.md)。以下のevidenceリンクはローカル検証履歴であり、リポジトリには含めない。

state: done
worklog: enabled
tier: Full
plan: plan_20260907_irodori_v4

2026-09-08: [利用者がv4.1採用を承認](evidence/user-acceptance-v41.md)。旧資源比較gateは診断へ変更し、聞き取れる品質の許容をE4として再受入する。新規既定v4.1/benchmark gateの限定実装と独立再受入はpass。既存設定保持・数値/共存証拠を保持し、利用者による実UIの総合動作確認と最終承認を受領した。全REQ受入pass、計画完了。旧未達は[round14](evidence/wp5-acceptance-round14.md)に保持。

## 受入表

| ID | 要件概要 | 検証方法 | 証拠種別 | WP | 実装状態 | 証拠 | 受入 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| REQ-01 | 固定環境とモデル一式 | manifest/lock/export再現 | E1/E3 | WP-1 | implemented | [最終manifest](evidence/wp1-manifest.json)、[固定入力検証](evidence/acceptance-fixed-inputs.json)、[再export](evidence/acceptance-reexport-parity.json)、[手順](../../tools/irodori_export/README.md) | pass（[独立受入](evidence/wp1-acceptance.md)） |
| REQ-02 | tokenizer一致 | 公式ids/mask fixtures | E1 | WP-2 | implemented | [独立比較](evidence/wp2-acceptance-parity.json)、[独立Goテスト](evidence/wp2-acceptance-go-test.log)、[実装報告](evidence/tokenizer-parity-worker.json) | pass（[独立受入](evidence/wp2-acceptance.md)） |
| REQ-03 | v4.1 ONNX graph | CPU/CUDA parityと欠損検査 | E1 | WP-1/3 | implemented | [独立全graph](evidence/parity-acceptance.json)、[Go numeric parity/CPU-CUDA smoke fix worker](evidence/wp3-worker.json)、[WP-3初回受入](evidence/wp3-acceptance-round1.md)、[fix1受入](evidence/wp3-acceptance-round2.md)、[fix2受入](evidence/wp3-acceptance-round3.md)、[fix3受入](evidence/wp3-acceptance-round4.md) | pass（WP-1/3。CPU/CUDA 6 graph、manifest/fixture validation、bundle loadを独立確認） |
| REQ-04 | 独立条件とduration/CFG | 4条件/境界tensor検証 | E1/E2 | WP-3 | implemented | [Go条件/CFG/duration実装](evidence/wp3-worker.json)、[fix4受入](evidence/wp3-acceptance-round5.md)、[WP-3最終受入](evidence/wp3-acceptance-final.md)、公式Python実測: evidence/v4.1-runtime.json | pass（製品14 features、raw/frame、4条件、CFG、CPU/CUDAを独立確認） |
| REQ-05 | アプリの音声生成契約 | 実WAV/文分割統合 | E1/E2 | WP-3/4 | implemented | [WP-3最終受入](evidence/wp3-acceptance-final.md)、[WP-4 worker](evidence/wp4-worker.json)、[WP-4初回受入](evidence/wp4-acceptance-round1.md)、[WP-4 fix1受入](evidence/wp4-acceptance-round2.md) | pass（公開API、複数文、300ms/3秒、全失敗拒否、48k mono PCM16を独立確認） |
| REQ-06 | 設定保持/事前検査/復帰 | 設定と異常系試験 | E1/E2 | WP-4 | implemented |[WP-4 worker](evidence/wp4-worker.json)、[WP-4初回受入](evidence/wp4-acceptance-round1.md)、[WP-4最終受入](evidence/wp4-acceptance-final.md)、[設定保持テスト](../../internal/store/store_test.go)、[採用後の独立受入](evidence/wp5-adoption-acceptance.md) | pass。旧固定Wailsでの利用者によるv4.1/v3設定保持確認を保持。新規既定v4.1の保存/再読込、既存v3/v4.1/任意パス・RefWAV/narratorDir保持、空/欠落時の既定適用を隔離公開APIで独立確認。 |
| REQ-07 | ORT/BGM共存と取消 | EP/取消/連続運転 | E1/E2 | WP-4/5 | implemented | [WP-4 worker](evidence/wp4-worker.json)、[WP-4初回受入](evidence/wp4-acceptance-round1.md)、[WP-4 fix1受入](evidence/wp4-acceptance-round2.md)、[WP-4 fix2受入](evidence/wp4-acceptance-round3.md)、[WP-4最終受入](evidence/wp4-acceptance-final.md)、[WP-5 fix3受入](evidence/wp5-acceptance-round4.md) | pass（[round5](evidence/wp5-acceptance-round5.md)、[round6](evidence/wp5-acceptance-round6.md)。CUDA/CPU/強制auto fallback各3周期、共有BGM/Talk、取消順序、同Service再生成を独立確認。正式CUDA40step全文3周期も[round12](evidence/wp5-acceptance-round12.md)で確認） |
| REQ-08 | 性能とメモリ | 10原稿/20反復/期限計測 | E2 | WP-5 | implemented |[短条件診断 v3](../../evidence/tts-benchmark-steps2/benchmark-cuda-v3-seed0.json)、[短条件診断 v4.1](../../evidence/tts-benchmark-steps2/benchmark-cuda-v4.1-seed0.json)、[WP-5初回受入](evidence/wp5-acceptance-round1.md)、[fix1受入](evidence/wp5-acceptance-round2.md)、[fix2受入](evidence/wp5-acceptance-round3.md)、[fix3受入](evidence/wp5-acceptance-round4.md)、[採用後の独立受入](evidence/wp5-adoption-acceptance.md) | pass（改訂採用基準）。正式170行を独立再評価し、件数/条件/生成成功/v4の60秒期限を確認。現行gateの全phase Error・child failure・最終write後guardを独立pass。p95比1.93365、v4 last5最大18,771MiB、Talk05欠測991msと旧fail/exit1は診断・履歴として保持。 |
| REQ-09 | 読みとナレーター品質 | 利用者の総合品質許容 | E4 | WP-5 | implemented |[60 WAV短条件manifest](../../evidence/tts-audition-steps2/audition-manifest-cuda.json)、[WP-5初回受入](evidence/wp5-acceptance-round1.md)、[fix1受入](evidence/wp5-acceptance-round2.md)、[fix2受入](evidence/wp5-acceptance-round3.md)、[fix3受入](evidence/wp5-acceptance-round4.md)、[採用後の独立受入](evidence/wp5-adoption-acceptance.md) | pass（E4）。[利用者回答](evidence/user-acceptance-v41.md)によりv3/v4.1とも聞き取れるアナウンス品質を許容。正式60 WAVの全hash/形式passを保持。全30ペア個別採点済み・誤読0とは記録しない。 |
| REQ-10 | E2E/ビルド/文書 | core E2Eと回帰 | E1/E2/E3/E4 | WP-5 | implemented |[限定local fixture E2E](../../evidence/tts-e2e/e2e-cpu.json)、[WP-5初回受入](evidence/wp5-acceptance-round1.md)、[fix1受入](evidence/wp5-acceptance-round2.md)、[fix2受入](evidence/wp5-acceptance-round3.md)、[fix3受入](evidence/wp5-acceptance-round4.md)、[採用後の独立受入](evidence/wp5-adoption-acceptance.md) 、[最終利用者承認](evidence/user-acceptance-final.md)、[完了受入追補](evidence/acceptance-completion.md) | pass。正式core E2E・全Go/race・Wails/frontend build・文書deltaの独立検証を保持。利用者がアプリ起動と一通りの動作確認を行い、問題なしとして現状を最終承認。個々の操作ログや実行EXE hashを別途受領したとは記録しない。 |

## 調査証拠（実装・受入とは別）

- 公式v4/v4.1 CUDA、現行v3 CUDA実行: 成功。7件のWAV検証成功。
- tokenizer比較: 6/6不一致。既存exporter: 実モデルwrapper構築失敗を再現。
- 公式Python依存: 3.12失敗→3.11成功。UTF-8出力指定で絵文字ログ成功。
- `go test ./internal/localtts/... ./internal/audiofmt/... ./internal/store/...`: 成功。localttsにはテストなし。
- ユーザーによる試聴用4件の事前品質確認は完了（E4）。製品実装後の全要件受入は未実施。現在のWP-1受入とは区別する。

## 完了結果

[最終受入追補](evidence/acceptance-completion.md)によりREQ-01～10/WP-1～5の受入完了を確認し、accepting → done。新規設定はv4.1既定、既存設定は保持。必須自動検証の有効証拠と利用者の品質・実UI総合承認を揃えた。未完了要件・追加実装・人間回答待ちはない。以下のWP別記録は各時点の履歴として保持する。

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
- WP-3実装結果（2026-09-08 fix worker）: manifest schema-v2の6 graphを名前/I/O/type/shape/external dataまで検証し、v3 metadata.json経路をlegacy分岐で保持。v4はModernBERT text/caption、speaker patch=4のlatent後floor/truncate、dynamic codec padding、公式linear schedule/text normalization、全14 duration features、独立CFG、decoder後seconds crop、48 kHz mono PCM16出力を実装。[worker証拠](evidence/wp3-worker.json)
- WP-3 fix workerのCPU/CUDA Go/ORT numeric parity（6 graph、4条件、cfg=1/default、40step）とsmoke、expanded関連テストは成功。独立再受入、WP-4/5のサービス統合・品質・性能・E2E・既定切替は未実施。Acceptance欄と計画stateはfix workerから未変更、全計画stateはimplementingを維持。
- WP-3 fix2実装結果: v4製品textを`normalize_text(raw).strip()`相当の単一境界へ統一し、tokenizer/duration consumerの空白同値テストを追加。`cmd/tts-parity` は [product pipeline harness](../../internal/localtts/irodori/pipeline/parity.go) 経由で公式fixtureを実際の条件batch、CFG、linear schedule/sampler、duration、codec経路へ注入する。fixtureは固定source/model/tokenizer/codec/lock/generator hashを記録し、隔離再生成hash一致を確認。manifest graph/fixture hashを必須化し、正常bundleのgraph contract変異負例を追加。[fix2証拠](evidence/wp3-fix2-worker.json)

## WP-3 fix3 実装結果（2026-09-08）

- 通常製品 bundle の `LoadManifest`/`Validate`/`VerifyHashes` は検証専用 go-parity fixture の同梱を要求しない。parity 実行時の `VerifyFixture` は明示/default fixture の存在、manifest SHA256、公式 provenance と各 hash を必須検証する。[fix3 worker 証拠](evidence/wp3-fix3-worker.json)
- 製品 synthesis と parity harness は共通 `durationFrames`（raw→expm1→durationScale→round/clamp、seconds frame）および `runDenoising`（linear schedule、CFG、40 step）を通り、fixture の14 features/条件 tensorを同じ product 分岐へ注入する。CLI に `--duration-scale`、`--fixture-case`、明示 `--seconds` を追加した。
- targeted/expanded/全 Go test、CPU/CUDA 6 graph・4条件・CFG・40 step parity、scale=0.5/2、seconds=0.5、製品 0.5 秒 smoke は成功。Acceptance/state は変更せず、WP-4/5 は未実施。








## 利用者採用後の差分（2026-09-08）

- [承認](evidence/user-acceptance-v41.md)に従いREQ-08/09を改訂した。旧比較fail・旧exit1・測定欠測はround14と元JSON/CSVに保存する。
- 新規既定v4.1、既存設定保持、benchmark採用判定の限定実装を独立検証した。全phase生成失敗・child失敗と終了判定の整合、最後のwrite後のprovenance確認もpass。
- 常駐Runtimeの追加最適化は不要。実UIの総合受入は今回の最終承認により完了した。



