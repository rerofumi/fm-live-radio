# TTS benchmark / E2E（2026-09-08）

## 成果物の保管

実行ログ・WAV・JSON/CSV・作業レポートは `/evidence/` または `/docs/**/evidence/` にローカル保存し、リポジトリへ追加しない。共有する結果は計画の受入要約へまとめる。以下の個別evidence参照はローカル検証履歴。

## 目的と既定

新規設定の既定Irodoriは `model/irodori-v4.1`。保存済みv3・任意パスは保持する。検証CLIではmodel/EPを明示する。ORTはプロセス内で初期化条件を固定するのでCPU/CUDA/autoを同一プロセスで切り替えず、別プロセスで実行する。

## 固定原稿と測定

`internal/ttseval` の10原稿は各200–300 runeで、数字・固有名詞・記号を含む。`mise run tts-benchmark` は製品 `localtts.Service` の1 Talk経路（Talk内でRuntimeを文間再利用）をv3/v4.1同じref/seed/EPで処理し、preflight/load/各文/結合/closeを実イベントで分離する。各原稿の全体・各文、RTF、明示 `--deadline-ms` と期限判定、生成WAV SHA256をJSON/CSVへ出力し、JSONの `adoption` に正式件数・条件整合・全phase生成成功・v4期限・child failureを反映する。p95比は `comparison_diagnostics`、VRAM実値・旧比較判定・欠測は子reportの `rows` / `steady_summary` に保存し、採用判定や終了コードを失敗させない。

VRAMは推論中に `nvidia-smi --query-compute-apps=pid,used_memory` を100ms間隔でpollし、対象processのpeakを採る。parse不能、PID欠落、権限不可は0ではなく `unavailable` と理由を記録する。20回定常反復では最初5回の中央値、最後5回のrange/max、最初の中央値+512 MiBという旧比較基準の診断結果を保存する。品質の読み・声質・自然さは自動判定しない。

全60 WAV（10原稿×seed 0/1/2×v3/v4.1）を作る場合は、製品Serviceの全文40stepを使い、seed集合は厳密に `{0,1,2}` とする。同じ `--ref` を使い、file/hash/text/expected reading/model/seedを人間試聴manifestとして使用する。steps=2は診断用で、正式な聴取成果物ではない。

固定試聴セットは、tracked treeへWAVを置かない専用出力先を指定して `mise x -- go run ./cmd/tts-benchmark --audition-only --out evidence/tts-audition-formal --ep cuda --steps 40 --seconds -1 --scripts 10 --steady-repeats 20` で生成する（`--audition-only` は60件とmanifestを作り、失敗時は非zero）。WAVはignore対象、manifest/手順だけを証拠として保存する。既存の `evidence/tts-benchmark-steps2` にあった既知のsteps=2生成WAV 20件は運用上削除済みであり、再生成しない。JSON/CSV診断レポートは保持する。

JSON/CSV reportには実行時のjj snapshot/parent（no-patch templateで取得した40桁ID）、steps/seconds/CFG/scale、model/ref/input hash、ORT/EP/GPU/driver、v3 metadata/tokenizer/graph hashを記録する。`--deadline-ms` は製品の次Talkスケジュール予算（reportの `talk_deadline_source`）として明示し、正式件数・条件不整合、生成失敗、v4 Talk/steady期限超過、child失敗は非zeroになる。VRAM欠測やfirst5中央値+512MiB超過は診断値として保持し、採用失敗とはしない。last5 rangeは診断値であり合否条件ではない。WDDMでprocess `used_memory=N/A` の場合は常駐Windows collectorを使い、DXGI LUID/GPU UUIDと厳密に一致する対象PIDのCIM `DedicatedUsage`を取得する。device-totalは正式process値へ代入しない。途中error/EOF/欠測/staleは永続的にunavailableとし、内部monotonic時刻で連続性を確認する。文ごとの区間peakとTalk全体のStop/join後peakを分離し、OS Killの所有者は一つにする。

## Local fixture E2E

`mise run tts-e2e` は `httptest` のRSS三項目とOpenAI互換chat fixtureを入力に使うが、BGMは実Stable Audio 3、Talkは製品 `talk.Service`、切替は実 `player.Player` を通す。各周期でBGM→Talk（先読みを含む）→audio/loudness URLを検査し、最低3周期を要求する。設定保存は `store.NewAt` のtemp directoryに隔離し、v4/v3それぞれを子プロセスで再読込・生成する。unknown schemaとbundle欠損はTalk開始前に拒否し、Skip/取消/再生成と実inference start/end、join、close、active shutdownを記録する。Stable Audioモデルが無い・生成不能な場合は固定BGMへフォールバックせず、非zeroで環境ブロックを報告する。

実WebViewの再生可否、声質/自然さ、数字の読みの良否はこのE2Eのpassには含めず、人間手順へ引き渡す。

## Commands

```powershell
mise run tts-smoke
mise run tts-e2e
mise run tts-benchmark
mise x -- go run ./cmd/tts-benchmark --ep cuda --seed 0 --scripts 10 --steady-repeats 20
```

## 2026-09-08 独立受入で確定した検証器の境界

- 親開始時のjj snapshot/parentと、所有出力だけを除いたdiff hashを固定する。各子の開始/終了、親の結合後で実parent/current hash/captured snapshot diff hashを照合し、レポート生成の自己干渉と実コード変更を区別する。架空hashや旧snapshotと現hashの混合は拒否する。
- repo内出力はevidence配下または既存ignored build/bin配下を使用する。別volumeの外部出力も可能で、製品sourceをhash除外しない。長い正式runはbuild/bin/acceptanceへ隔離し、tracked編集を止める。
- scripts=1/steady-repeats=1は短診断として実行できるが、正式10原稿/20反復不足は必ずnonzero。短条件の比率・WAVを正式性能/品質に代用しない。
- 既定60,000ms期限は30秒BGM×3曲のうち2曲目返却前にTalk先読み開始→残り2曲分から導出。gap/生成待ちは予算へ加えない。任意指定は明示条件として記録する。
- 証拠: docs/plan_20260907_irodori_v4/evidence/wp5-acceptance-round13.md。生成器と計測器の独立passであり、利用者承認による採用条件改訂と独立再評価は90_statusおよびevidence/user-acceptance-v41.mdを参照する。




