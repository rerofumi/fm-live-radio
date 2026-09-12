# ローカル生成の調停・先読み

確認日: 2026-09-12。対象: Windows amd64、Go 1.26.1、Node 24.14.1、Wails 2.12.0。以下は、実装前の現行事実と、WP-1〜3実装後に独立受入で確認した as-built を分けて記録する。12GB実機のVRAM/速度や実GPU E2Eの成否は本資料から判断しない。

## 実装前の現行事実（2026-09-12 計画時点）

| 確認箇所 | 当時の事実 | 設計上の境界 |
| --- | --- | --- |
| [`generation/queue.go`](../../internal/generation/queue.go)、全Go呼出、[`domain/types.go`](../../internal/domain/types.go) | `NewQueue` は semaphore 型だが生成経路から未使用。`MaxWorkers` は設定/既定値にあるだけ。 | 設定を1へ変えるだけでは Music/Talk は直列化されない。 |
| [`localtts/service.go`](../../internal/localtts/service.go) の旧 `synthesizeWav` / `synthesizeSentences` | mutex は TTS Service 内だけ。1 Talk の Runtime は複数文で再利用し最後に Close。 | Service 間で Runtime load〜Close を覆う共有枠が必要。 |
| 旧 `synthesizeToFile`、[`musicgen/service.go`](../../internal/musicgen/service.go) の旧 `Generate` | ctx 取消後も実 worker の終了を receive で待ち、その後 defer Close。 | 取消要求だけで枠を返さず、join→Close→解放を維持する。 |
| 旧 [`player/player.go`](../../internal/player/player.go) の `pickBGM`、[`app.go`](../../app.go) の旧 `PrefetchTalk` | Talk/Music の先読みを順に別 goroutine へ渡すが、受付順は保証されない。ready がなければ同期生成し、先読みとの合流もない。 | 起動順だけでなく、同一契機の予約順と同じ需要の共有が必要。 |
| 旧 Player / [`musicgen/cache.go`](../../internal/musicgen/cache.go) | `prefetchedMusic` は1件。`CacheLimit` はディスク WAV 件数で先読み数とは別。ジャンル変更は旧先読み/生成中結果を維持し、fallback は WAV の生成ジャンルを調べない。 | 2曲 FIFO、genre epoch、sidecar provenance は現行仕様変更として別途検証する。 |
| 旧 cache / [`audio/server.go`](../../internal/audio/server.go) | `TrimCache` の保護は `keepPath` 1件。audio token は path を参照するが読み取り中の削除保護はない。 | 複数 protected path と URL read の request-scoped reference が必要。 |

この時点の確認はコード構造の調査であり、12GB環境の停滞原因を実測で確定したものではない。新規外部ライブラリ/APIの採用もない。

## 実装後 as-built（WP-1〜3、2026-09-12）

### 生成枠・優先順・取消

- [`generation.Arbiter`](../../internal/generation/arbiter.go) はプロセス共有の容量1。`Reserve` / `(*Arbiter).Acquire` が予約と lease を分け、Music→Talk の優先と同種 FIFO を実装する。
- `generation.WithReservation` と `ReservationFromContext` は Player が登録した予約をサービスへ一度だけ転送する経路。直接 `musicgen.Service.Generate` / `localtts.Service` を呼ぶ場合はサービス側が予約を作る。
- `musicgen.Service.Generate` と `localtts.Service` は Runtime load 直前から推論完了後の `Close` 完了まで lease を保持する。待機中の取消は load せず、許可後の取消は worker join 後に解放する。`maxWorkers` / provider は共有枠を増やさない。
- `Player.PrefetchNext` は同一契機で Music reservation を同期登録してから Talk worker を登録する。Music の entry gate を先に通すため、goroutine の起動順だけに依存しない。
- `Player.NextItem` / `PrefetchMusic` は同期取得と先読みを同じ Talk/Music job に合流させる。Skip・設定変更・Shutdown は公開 owner を無効化し、実 worker の join 完了までは busy を保持する。`Player.Shutdown` は cancel→prefetch worker join→work join の順で完了する。

### 2曲 FIFO と Talk 遅延

- [`Player.musicReady`](../../internal/player/player.go) は未再生 ready BGM の FIFO。ready 曲＋未完了予約の合計を最大2件に保つ。
- 最初の1曲は準備でき次第 `NextItem` から返し、消費または明示 `PrefetchNext` / `PrefetchMusic` を補充境界として不足分だけを逐次補充する。`CacheLimit` はディスク上の WAV 件数であり、この2曲枠とは独立する。
- Talk が slot に到達しても Talk job が未完成なら slot/history を消費しない。ready BGM を返し、BGM も空なら同じ Music job を待つ。Talk 完成後の次境界で Talk を採用する。
- Talk 失敗はその slot を一度消費して BGM fallback へ進む。同じ境界での即時再試行は `talkFailureBlocked` / `musicFailureBlocked` で抑止し、明示ヒントまたは消費を次の retry boundary とする。

### ジャンル世代・fallback・ファイル保護

- genre-only の [`Player.UpdateStableAudio3Genre`](../../internal/player/player.go)、`UpdateConfigFromSave` は `musicEpoch` だけを進め、music FIFO/旧音楽予約を無効化する。現在 item、Talk、記事履歴、再生カウンタは維持し、正規化後の同値保存はリセットしない。
- 音楽 worker は config snapshot・epoch・owner を持つ。`pickBGM` は登録直後と最終公開直前に generation/closed/epoch/genre を検査し、不一致時は URL を `ReleaseAudioURL` して最新 epoch で選び直す。A→B→C / A→B→A の旧結果を公開しない。
- [`musicgen.MetadataPath`](../../internal/musicgen/cache.go) の sidecar（`<wav>.json`）に生成時 genre を atomic 保存する。`PickFallbackForGenre` は正規化済み genre と一致する有効 sidecar の WAV だけを返し、不明/破損/不一致 provenance は候補外とする。fallback 結果に現在 genre を上書きして証明することはない。
- [`musicgen.TrimCache`](../../internal/musicgen/cache.go) は `fileprotect.Protected` と複数 keep path を確認し、保護中 WAV を飛ばす。削除する WAV の sidecar も同時に削除し、保護数が上限超過中は整理を延期する。
- [`audio.Server.RegisterFile`](../../internal/audio/server.go) は token TTL と fileprotect 参照を登録する。`/audio/<token>` の ServeFile 中は request-scoped reference を保持し、`ReleaseAudioURL`、TTL失効、`CleanupExpired`、`Close` で token/envelope と登録参照を解放する。

## 確定した実装APIと再検証コマンド

主なAPIは次のとおり。

- 調停: `generation.NewArbiter`, `generation.SharedArbiter`, `Arbiter.Reserve`, `Arbiter.Acquire`, `Reservation.Acquire`, `Reservation.Release`, `Lease.Release`, `generation.WithReservation`。
- Player: `New`, `NextItem`, `Skip`, `PrefetchNext`, `PrefetchMusic`, `UpdateStableAudio3Genre`, `UpdateConfigFromSave`, `Shutdown`, `Status`。
- cache/保護: `musicgen.PickFallbackForGenre`, `musicgen.TrimCache`, `musicgen.ProtectResult`, `fileprotect.Acquire`, `fileprotect.Protected`。
- audio: `Server.RegisterFile`, `Server.ReleaseAudioURL`, `Server.CleanupExpired`, `Server.Close`, `Server.LoudnessURLForAudioURL`。

WP-4最終回帰で実行するコマンドは以下。`mise install` は最初に実行する。実環境ではWinGet shimが起動できなかったため、同じmise実体を絶対パスで呼び出している。

| ID | コマンド | 結果 |
| --- | --- | --- |
| C01 | `mise -C <workspace> x -- go test -count=1 ./...` | exit 0 |
| C02 | `mise -C <workspace> x -- go test -race -count=1 ./internal/generation ./internal/player ./internal/musicgen ./internal/localtts ./internal/talk ./internal/audio ./internal/fileprotect` | exit 0 |
| C03 | `mise -C <workspace> x -- npm --prefix frontend run build` | exit 0 |
| C04 | `mise -C <workspace> run build` | exit 0。既知の `.rsrc merge failure: multiple non-default manifests` を linker が出力するが、Wails/EXE生成は完了。 |

既存の実モデル opt-in test は環境変数未指定のため skip（`FM_RADIO_IRODORI_MODEL` / `FM_RADIO_IRODORI_REF`、または `FM_RADIO_GPU_COLLECTOR_E2E` / `FM_RADIO_GPU_SAMPLER_E2E`）。新規の調停・FIFO・epoch・cache・token test に skip はなく、fake/barrier と TempDir を使う。`cmd/tts-benchmark` のGPU process probeや統合benchmarkは明示環境変数が必要で、本WPの実GPU E2E成功・12GB性能改善の証拠には扱わない。

## 既知の制約

- 12GB実機のVRAM量・生成速度・症状解消は未測定。shared arbiter は同時実行を抑えるが、単独モデルの資源不足や外部プロセスのGPU利用を解決するものではない。
- Talk が遅く BGM 2曲を使い切った場合、またはジャンル変更直後に一致する生成済み WAV がない場合は待機が発生し得る。常時無停止は保証しない。
- sidecar を持たない既存 WAV は削除しないが、ジャンルを検証できないため genre fallback 候補にはしない。
- C04は終了コード0だが、Wails linker の既知 manifest 出力が残る。実UI起動の追加確認はこの回帰結果に含めない。

関連する正本は [current requirements](../requirement.md)、[current specification](../specification.md)、[生成制御計画](../plan_20260912_generation_control/plan.md)、[status](../plan_20260912_generation_control/90_status.md) である。
