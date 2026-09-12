# 生成リソース制御ステータス

state: done
worklog: enabled
tier: Light
plan: plan_20260912_generation_control
e2e: disabled

要件・設計の正本: [plan.md](plan.md)。2026-09-12 に全WPの実装・統合、最終独立受入、as-built文書反映を完了。REQ-01〜08はすべてpass。

## 受入表

| ID | 要件概要 | 検証方法 | 証拠種別 | WP | 実装状態 | 証拠 | 受入 |
| --- | --- | --- | --- | --- | --- | --- | --- |
| REQ-01 | 全生成経路のロード～Close 排他 | T-01/T-03、サービス呼出経路照合 | E1/E3 | WP-1/2 | implemented | WP-1の`Arbiter`/サービス境界に加え、Playerの`TalkGenerator`/`MusicGenerator`需要ジョブが各サービスを1回だけ呼び、予約をcontextで転送する実装を`internal/player/player.go`（`startTalkPrefetchLocked`/`startMusicPrefetchLocked`）へ統合。`TestNextItemJoinsTalkPrefetchDemand`/`TestNextItemJoinsMusicPrefetchDemand`で同期取得と先読みの同一需要を1呼出に統合。対象パッケージ通常/raceはexit 0。 | pass（第1段階 fix round 2、WP-1/2独立受入） |
| REQ-02 | 待機音楽優先、同時契機の受付順 | T-02、起動/受付イベント照合 | E1 | WP-1/2 | implemented | `internal/player/player.go`の`PrefetchNext`がMusic reservationを同期受付し、Music jobのentry gate解放後にTalk jobを開始。`TestPrefetchNextStartsMusicBeforeTalk`は同一契機のGenerate entryをorder channelでMusic→Talkと検証し、逆順ならfailする。Arbiterのmusic→talk優先、同種FIFO、サービスの`generation.WithReservation`転送はWP-1証拠を保持。 | pass（第1段階 fix round 2、WP-1/2独立受入） |
| REQ-03 | 取消/失敗/Shutdown 後の解放・回復 | T-03/T-04、既存取消回帰 | E1/E3 | WP-1/2 | implemented | `Player`の実Talk/Music worker/job/context経路を`TestRunningTalkCancellationJoinsAndProtectsReplacement`、`TestRunningMusicCancellationJoinsAndProtectsReplacement`、`TestRunningTalkAndMusicShutdownCancelJoinAndRejectOldResults`でbarrier検証。Skip/UpdateConfig/Shutdownのcancel後、join前busy、join後busy=0、旧結果のready/public item/history非混入、置換owner保護を確認。worker join後にreservationを解放し、`internal/talk/talk.go`の予約deferがRSS/LLM/TTS早期失敗の未消費予約を解放。 | pass（第1段階 fix round 2、WP-1/2独立受入） |
| REQ-04 | 未再生2曲の先読みと単一需要 | T-04/T-05 | E1 | WP-3 | implemented | `internal/player/player.go` の `musicReady` FIFO と epoch別 job 管理で ready＋pending を最大2に制限。consume/hint/明示NextItemを補充境界にし、失敗した補充は `musicFailureBlocked` で同一サイクルの即時再試行を停止。`TestMusicPrefetchFillsTwoFIFOAndRefillsAfterConsume`、`TestMusicPrefetchDemandBurstKeepsTwoAndRefillsAfterEachConsume`、`TestMusicFailureStopsRefillUntilExplicitHint`を通常/raceで通過。 | pass（WP-3 boundary round 3独立受入） |
| REQ-05 | 遅延 Talk を待たず ready BGM を返す | T-06 | E1 | WP-3 | implemented | `NextItem` は pending Talk がある場合、ready BGM の有無にかかわらず既存または新規 Music 需要へ `pickBGM` で合流する。Talk workerを継続し、BGM返却ではslot/historyを更新しない。Talk failureは失敗slotを一度だけ消費してBGMへフォールバックし、再生成を同境界で起こさない。`TestPendingTalkWaitsForMusicWhenNoReadyBGM`、`TestPendingTalkKeepsSlotAndHistoryUntilTalkBoundary`、`TestTalkFailureConsumesSlotOnceAndFallsBackWithoutImmediateRetry`、`TestTalkDisabledCycleOneWaitsForSameMusicDemandWhenBGMEmpty`を通常/raceで通過。 | pass（WP-3 boundary round 3独立受入） |
| REQ-06 | 次 BGM の新ジャンルと旧世代排除 | T-07/T-08 | E1/E3 | WP-3 | implemented | `internal/player/player.go` の `pickBGM` を bounded loop 化し、RegisterFile 後の最初の epoch/genre check 通過後から最終 publish 直前までを注入可能な unexported hook で再現。最終 gate は `p.mu` 下で generation/closed、`musicEpoch == selectionEpoch`、result/current normalized genre を同時再確認し、不一致時は登録 URL を `ReleaseAudioURL` して最新設定の需要へ戻る。`internal/player/player_test.go` の `TestMusicGenreChangeAfterFirstEpochCheckReleasesAndRechooses` は旧 URL release・新 genre item・旧 ready/status 非混入を検証し、`TestMusicGenreMultiEpochIdentityRejectsLateResults` は reservation context の Acquire/lease 解放を含む A→B→C と A→B→A を検証。`mise x -- go test ./internal/player -count=1`、race、関連通常/race、全 Go が exit 0。 | pass（WP-3 final fix round 4独立受入） |
| REQ-07 | ジャンルを検証する fallback と参照保護 | T-08/T-09 | E1 | WP-3 | implemented | `internal/musicgen/cache.go` のsidecar provenance一致fallback、複数protected pathを飛ばすTrimCache、WAV/sidecar同時整理を実装。`internal/audio/server.go`/`internal/fileprotect` はtoken TTL、Close、明示Release、HTTP read scoped referenceを保持し、`CleanupExpired(now)` と注入可能clockで時刻/GCを決定化。fallback、複数保護、延期後cleanup、無関係file、URL GET/Release/TTL/Closeを通常/raceで通過。 | pass（WP-3 boundary round 3独立受入） |
| REQ-08 | 回帰/ビルド/current docs | T-10、C-01〜04、受入後の文書照合 | E1/E3 | WP-2/4 | implemented | WP-4で [current requirements](../requirement.md)、[current specification](../specification.md)、[generation-scheduling cheatsheet](../cheatsheet/generation-scheduling.md)、[cheatsheet index/link](../cheatsheet/index.md) を2026-09-12 as-builtへ更新。最終独立受入でC-01全Go、C-02対象race、C-03 frontend、C-04 Wails buildは全てexit 0。C-04は既知の `.rsrc merge failure: multiple non-default manifests` を出力するがEXE生成完了。新規制御testのskipなし。 | pass（final acceptance、as-built文書照合完了） |

実装者は実装状態と証拠を更新し、受入欄と done は別セッション/人間が判断する。調査時の baseline 成功を新要件の実装証拠に転記しない。

## 自動検証ケース

新規試験は GPU/モデル/外部 RSS/LLM に依存させない。Player/サービスに小さな生成器・Runtime 注入点を設け、製品が通る予約・解放・公開処理をそのまま試す。sleep による発生確率ではなく channel/barrier で順序を作る。新規制御テストは skip 不可。

| ケース | 対象・手順 | 期待結果 |
| --- | --- | --- |
| T-01 | 複数 Service、同期/先読みの Music/TTS を競合させ、fake Load/Run/Close にゲートを置く。各 provider と maxWorkers > 1 も入力する。 | Close 完了前に次の Load が始まらず、全 active 区間合計は最大1。サービスごとの別枠にならない。 |
| T-02 | 既存ジョブで枠を保持し Talk→Music の順に待機登録して解放。同種複数件、空き枠で同じ Player/App 契機、実行中 Talk に後着 Music も試す。 | 待機 Music→Talk、同種 FIFO。同時契機は Music が先に受付/開始し、実行中 Talk は中断しない。呼出順だけを検査するテストにしない。 |
| T-03 | 待機前/待機中/許可と同時の取消、既存 deadline、Load 失敗、推論失敗、取消後に遅れて終わる Run、遅い Close、未消費予約の早期エラーを注入。 | 取消済みは新規 Load なし。既に許可済みなら必ず Close 後に解放。待機列/枠が漏れず、後続が開始する。取消要求だけで枠を解放しない。 |
| T-04 | 同じ需要に NextItem/先読みを重ね、Skip/設定変更/Shutdown 中の worker と置換 worker を交差させる。 | Talk の RSS/LLM/TTS と BGM が需要ごとに1処理。旧結果は未公開、旧 worker は置換予約を消さず、置換なしの取消完了は busy を残さない。Shutdown 完了時は待機/稼働0。 |
| T-05 | 空→最初の曲返却→2枠まで補充→消費→再補充。補充失敗、同時ヒント連打、CacheLimit=1 も実行。 | 1曲目から返却可能。ready＋未完了予約は最大2、余分な予約なし、1曲消費で不足だけ補充。ディスク上限と2枠を混同しない。 |
| T-06 | Talk を barrier で止め、ready BGM を2件入れて Talk slot で NextItem。その後 Talk 完成/失敗、BGM 枯渇、Talk disabled/cycle=1 を試す。 | 未完成中は BGM を返し Talk を重複生成しない。完成後の次境界で Talk、履歴は採用時だけ。失敗は従来 fallback。全バッファ枯渇は同じ BGM 需要を待つ。 |
| T-07 | ready旧曲＋稼働旧曲＋待機旧予約の各条件でジャンル変更。A→B→C/A→B→A、同値変更、Tuner/Settings 保存も試す。 | 旧キュー/予約を無効化、新 epoch のみ補充。遅い旧完了が結果/statusを上書きしない。再生中の URL、Talk、記事履歴はジャンル専用変更で破棄しない。 |
| T-08 | キャッシュから取り出した直後、URL登録中、最終公開直前に変更を挿入。生成失敗時は現ジャンル/旧ジャンル/metadataなし/破損 metadata の WAV を配置。 | 旧 epoch の未確定選択は最新で選び直す。一致 provenance のみ fallback、候補なしは正しいエラー。旧音声へ現在ラベルを付けない。 |
| T-09 | 実一時 WAV と sidecar を作り、生成→ready→選択→URL登録の各境界、TTL失効/参照解除、上限より多い保護ファイルで TrimCache。 | 受渡し中・有効URL参照中はWAV取得可能。上限超過は保護中だけ延期。解放後は古い未保護から整理し、無関係ファイルを消さない。 |
| T-10 | 既存設定/genre正規化/基本再生/gap/Skip/取消/世代・owner/保存済みv3/v4.1/ORT設定と新たな動作変更を照合。 | 意図した先読み・ジャンル契約変更だけを反映。既存テストの期待値変更は REQ と理由を記録する。 |

## 実装・受入で実行するコマンド

`mise install` を最初に実行する。既存 opt-in 実モデル test は有効化せず、skip 件数・理由を記録する。新規の排他・キャッシュ検証を skip で代替しない。

| ID | コマンド | 合格条件・タイミング |
| --- | --- | --- |
| C-01 | `mise x -- go test -count=1 ./...` | 全体の Go テスト成功。第1段階と最終受入で、変更対象の最新状態を検査する。 |
| C-02 | `mise x -- go test -race -count=1 ./internal/generation ./internal/player ./internal/musicgen ./internal/localtts ./internal/talk ./internal/audio` | 競合を作る新規テストが実行され、race/失敗なし。第1段階で未変更の audio は最終受入時に含める。 |
| C-03 | `mise x -- npm --prefix frontend run build` | TypeScript/Vite build 成功。 |
| C-04 | `mise run build` | Wails Windows build 成功。mise 定義の task を使用。 |

実装時は対象テストで反復し、段階の独立受入で上記を実行する。合格後、変更がなければ同じ全回帰を繰り返さない。`mise run tts-e2e`、性能ベンチ、実 GPU の手動確認は本計画の必須受入に追加しない。

## 計画時点の調査証拠

2026-09-12 / Windows amd64 / jj parent `6265717e`（Irodori v4.1 対応版）。

- `mise install`: 全定義導入済み。Go 1.26.1、Node 24.14.1、Wails 2.12.0。
- `mise x -- go test -count=1 -v ./internal/generation ./internal/player ./internal/musicgen ./internal/localtts ./internal/talk`: exit 0。Player 8件、musicgen 6件、localtts 3件が pass。実モデル取消テスト1件は必要環境変数未指定で skip、generation/talk は test なし。
- 同じ5パッケージの `mise x -- go test -race -count=1`: exit 0。現行テストの実行可否を確認しただけで、新しい同時生成制御の合格証拠ではない。
- frontend build: exit 0。`mise run build`: exit 0、Windows EXE 生成。linker の `.rsrc merge failure: multiple non-default manifests` 出力あり。実 UI 起動は未確認、警告ゼロとは扱わない。
- 全 Go suite C-01、実機の症状再現、将来仕様の T-01〜10 はこの計画作成時点では未実施。

## 検証の限界と未解決事項

- E2E 除外は利用者が選択済み。12GB 実機の速度・VRAM・症状解消は未検証であり、本計画の必須受入待ちにはしない。fake Runtime で制御を検証し、実装ポインタで実サービスへの接続を確認する。実機効果を後日確認する場合は利用者指定の条件で別途扱う。
- 外部 LLM や他プロセスの GPU 利用、単独モデルでの資源不足は本調停の対象外。2曲を使い切るほど Talk が遅い場合、またはジャンル変更直後は待機があり得る。
- 追加の利用者判断待ちはない。実装・受入証拠は受入表とworklogへ記録済み。

## 受入ログ

| 日付 | 担当 | 結果 | 対象・備考 |
| --- | --- | --- | --- |
| 2026-09-12 | persistent acceptance session | pass | 最終対象 `df0e15b40da334a24f9af40e40f088f3ae3a47e7`、parent `6265717e`、diff SHA256 `D97CE51DFC341140871B9B73C076345447632977CB7ABFAE54286C654FD3E0BD`。REQ-01〜08 pass、C-01〜04 exit 0、文書/リンク/scope確認済み。 |
