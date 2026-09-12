# 生成リソース制御 — Planning Worklog

worklog: enabled

## WL-001 — Step 0: 新規計画の開始

- 日付・担当: 2026-09-12 / Codex 計画作成セッション。
- 目的: VRAM 12GB 環境で音声・音楽の同時生成が停滞する問題の改善計画を作る。
- 入力・確認: 利用者の新規作成 / tier lite / worklog あり指定、[計画一覧](../plan_index.md)、完了済み [Irodori v4.1 計画](../plan_20260907_irodori_v4/90_status.md)、jj status。
- 決定: Tier Light、worklog 有効。利用者回答により実サービス E2E は対象外、自動テスト・ビルド中心。2曲分の先行生成とジャンル変更時の破棄を低優先度の第2段階に含める。
- 作成: この計画ディレクトリと worklog。開始時の jj は変更なし、親は 6265717e（Irodori v4.1 対応版）。
- 代替・範囲: 完了した移行計画への追記より、利用者が指定した独立した改善計画を採用。旧連番計画の移動・改名は今回の範囲外。
- 未確定: 音楽優先の境界を確認中。実機での速度・VRAM改善は未測定であり、数値目標は追加しない。
- 次: 現行生成・再生・キャッシュ経路の調査結果から要件と検証方法を具体化する。実装はこのセッションの対象外。

## WL-002 — Steps 1–4: 現行仕様・実装と検証環境の確認

- 日付・担当: 2026-09-12 / 同計画セッション。
- 目的: 排他制御を入れる単位、先読み容量、ジャンル変更の互換性を確認する。
- 入力: `internal/generation/queue.go`、`internal/localtts/service.go`、`internal/musicgen/service.go` / `cache.go`、`internal/player/player.go`、`app.go`、current docs と既存 cheatsheet。
- 結果: 共通 Queue は生成経路から未使用。TTS の mutex は Service 内だけ。Player は Talk→Music の順に別 goroutine を開始し、各 Runtime のロード～Close が重なる余地がある。音楽先読みは1件で、未完成 Talk は同期再生成される。ジャンル変更は先読みを保持し、fallback は WAV のジャンルを識別しない。
- 実行確認: `mise install` は全定義導入済み。Go 1.26.1 / Node 24.14.1 / Wails 2.12.0。対象5パッケージの `go test -count=1 -v` と `go test -race -count=1` が成功。実モデル取消テスト1件は opt-in 未指定で skip。generation/talk は現状テストなし。新要件の実装成功とは区別する。
- 決定: 利用者が待機中の音楽優先・実行中音声の完了待ちを選択。新規依存や外部 API は追加せず、既存 generation パッケージ内の調停と Player の先読み管理変更を計画する。
- 代替: 単純 mutex だけでは優先順が定まらない。文単位の解放では1 Talk 内に残る Runtime を保護できない。開始順の入替だけでも goroutine の到達順に依存するため採用しない。
- Step 1/2: Light のため Claim/App Requirements は独立ファイルを省略し plan.md へ集約。Step 3 は既存 domain primer を使用し、生成と再生の違い・資源の寿命を追記する。
- 未確認・次: 12GB 実機での VRAM・速度は未測定。E2E・性能ベンチを追加せず、制御の不変条件と fake Runtime による自動検証を要件化する。

## WL-003 — Steps 5–6: 要件・実装方針・検証方法の具体化

- 日付・担当: 2026-09-12 / 同計画セッション。
- 目的: 利用者が合意した制御と第2段階を、実装可能な要件へ落とす。
- 入力・確認: 利用者の非割り込み音楽優先の選択、[調査メモ](../cheatsheet/generation-scheduling.md)、現行取消・再生・ジャンル契約。
- 作成: [plan.md](plan.md) の REQ-01〜08 と WP-1〜4、[90_status.md](90_status.md) の T-01〜10 / C-01〜04。
- 決定: Runtime ロード～Close 完了の共通枠。同一先読み契機は音楽を同期予約。2曲は未再生分とし、Talk 未完成時は ready BGM へ進む。音楽専用 epoch で旧結果と旧ラベルの混入を防ぎ、ファイル参照の受渡し中も削除保護する。
- 代替: 二重生成を共通枠だけで順番待ちさせる案、2曲充足まで初回再生を待つ案、ジャンル変更時に Talk まで破棄する案を採らない。各案の理由は plan の該当方針に記録。
- 制約: 既存要求期限は待機込みで維持。単独モデルの資源不足、外部 LLM、2曲枯渇時の無停止、実機速度の数値保証は対象外。
- 次: 要件/受入表と文書リンク、実装未着手状態、計画一覧を照合する。

## WL-004 — Steps 7–8: 状態表・レビューと引継ぎ

- 日付・担当: 2026-09-12 / 同計画セッション。
- 目的: 次の実装担当が範囲と検証限界を確認できる状態にする。
- 作成・更新: [90_status.md](90_status.md) は全 REQ を todo / 未受入、plan state を planned に維持。Light のため別 review packet は作らず、plan の目的→範囲→要件→方針→判断事項を入口にする。
- 検証: frontend/Wails build はともに exit 0。Wails linker に既存コードの `.rsrc merge failure: multiple non-default manifests` 出力があり、生成 EXE の実 UI 確認はしていない。未解消の baseline 出力として status/cheatsheet に明記。
- 判断: E2Eなし・第2段階対象・音楽優先方式の利用者判断は完了。12GB実機の症状解消は未実測として残す。実機確認を今回の追加承認 gate にしない。
- 引継ぎ: 第1段階 WP-1/2→独立受入→第2段階 WP-3→最終回帰/独立受入→current docs 反映。実装・受入・as-built 更新（Steps 9–11）は今回の計画作成依頼の対象外で未着手。
- 最終確認: REQ 8件と受入表のID一致、planned/worklog有効/E2E無効、全件todo・未受入、worklog工程記録、ローカル参照42件の到達を確認。jj差分は計画・索引・調査メモの8文書のみ。製品ソース・依存定義の変更なし。

## WL-005 — Step 9: 実装開始

- 日付・担当: 2026-09-12 / leader セッション。
- 目的: 利用者承認済み計画を fm-dev-implement の役割分離で実装する。
- 入力: [plan.md](plan.md)、[90_status.md](90_status.md)、[生成調停の調査](../cheatsheet/generation-scheduling.md)、利用者の実装承認。
- 状態変更: `planned` から `implementing`。計画一覧も同期。
- 実行方針: fresh worker が各 WP を実装し、同一の独立 acceptance セッションが中間・最終受入を担当する。leader は製品コードを実装しない。
- 順序: WP-1（共通調停とサービス境界）→ WP-2（Player 統合）→第1段階中間受入→ WP-3（2曲先読み・ジャンル世代・cache）→ WP-4（文書反映）→最終受入。

## WL-007 — Step 10: WP-1 中間受入 round 1

- 日付・担当: 2026-09-12 / persistent acceptance セッション、leader 記録。
- 対象: snapshot `6570c36d0a0266b992e1c6b0f76a537c69848eeb`、diff SHA256 `E6DC1CA8527CFED3E7249041A5D349CD8FAEFC90F0042C10CFA3A3E8572030DB`。
- 独立検証: `mise -C <workspace> install`、generation/musicgen/localtts の通常テストと race が exit 0。WP-1 の6ファイル以外に製品差分・依存変更・不要cacheなし。
- 判定: shared arbiter接続、単体のmusic優先/FIFO、待機取消、music load失敗後回復は確認。REQ-01/03 は service境界のfake Runtime active/遅いClose/run失敗/TTS失敗回復のE1不足で fail。REQ-02は単体部分のみ確認、全体unverified。
- 次: fresh fix worker が不足するservice境界テストを追加し、同じacceptanceセッションで再受入する。Player/App範囲はWP-2まで先取り判定しない。

## WL-009 — Step 10: WP-1 中間受入 round 2

- 日付・担当: 2026-09-12 / persistent acceptance セッション、leader 記録。
- 対象: snapshot `0624f4bea4765633088ba55fbec782bae9aadcec`、diff SHA256 `55855A918362F74912205A29B32B8831ECFFE32D2C72535A9FA59A599EE9FD22`。
- 独立検証: `mise -C <workspace> install`、generation/musicgen/localtts の通常テストと race が exit 0。前roundのE3とarbiter単体結果は影響なしとして保持。
- 判定: REQ-01〜03のWP-1 service境界はpass。複数service、Load～遅いCloseの最大active=1、provider/maxWorkers非依存、優先/FIFO、待機取消、preflight/load/run失敗後の回復、転送予約の一回消費を確認。
- 残り: Player/Appの同一需要共有、同時契機のmusic先行予約、Skip/設定変更/ShutdownはWP-2で受入する。新たなmandatory fixなし。

## WL-011 — Step 10: 第1段階統合受入 round 1


- 日付・担当: 2026-09-12 / persistent acceptance セッション、leader 記録。
- 対象: snapshot `e8dba9f2700ca882776b459e1c71a1d2270313b8`、diff SHA256 `74B48CB1C1FA9D3D2B06BDF832FFD71857586B194F454A035D18F6A250856E50`。
- 独立結果: 対象パッケージrace、frontend build、Wails buildはexit 0。C-01全Goは未配置parity fixtureを必須参照する既存testでexit 1。Wailsの既知manifest linker出力は継続。
- 判定: WP-1 service境界の受入は保持。REQ-02は同一Player契機の順序testが逆順を検出できずfail。REQ-03は実workerを通すSkip/UpdateConfig/Shutdownのcancel/join/旧結果排除/busy解除E1不足でfail。REQ-08はC-01未達でfail。
- 次: fresh fix workerが順序・lifecycle試験を強化し、parity fixture testを外部部品なしで同じ契約を検証できる形へ直す。Acceptance欄はleader判断へ修正済み。

## WL-006 — Step 9: WP-1 実装結果

- 日付・担当: 2026-09-12 / WP-1 worker。
- 対象: REQ-01〜03 の共通生成枠・待機優先順・サービス境界。`internal/generation/arbiter.go` に容量1のプロセス共有 `Arbiter`（music優先、同種FIFO、待機取消除去、single-use reservation/context transfer）を追加し、`localtts.Service` / `musicgen.Service` の Runtime ロード直前〜Close後解放へ接続した。
- 注入点: `NewWithArbiter`、サービスの fake preflight/runtime loader。製品 `New()` は `generation.SharedArbiter()` を利用する。
- テスト: `internal/generation/arbiter_test.go` で容量1・music優先・同種FIFO・取消・予約一度消費、各サービス試験で待機取消時のロード未実行を channel/barrier で検証。sleep依存・skipなし。
- コマンド結果: `mise install` は実行環境のmise shim起動失敗で実行不能。代替として Go 1.26.1 の `gofmt`、`go test -count=1 ./internal/generation ./internal/musicgen ./internal/localtts`、`go test -race -count=1 ...` を実行し、いずれも exit 0（mise不可の詳細はworker報告）。
- 状態: REQ-01〜03 の実装状態を `in_progress` とし、Player統合・失敗/Shutdown回帰・同一契機の製品経路はWP-2へ残す。Acceptance列・REQ-08・計画文は変更していない。

## WL-008 — Step 9: WP-1 round 1 の不足テスト補完

- 日付・担当: 2026-09-12 / fresh fix worker。
- 目的: 独立受入で不足と判定された REQ-01〜03 のサービス境界証拠を、製品コードの挙動を変えずに補う。
- 変更: `internal/musicgen/service_arbiter_test.go` と `internal/localtts/service_test.go` に fake Runtime の Load〜遅い Close 区間を数える active/max 計測、複数 Service instance の共有 Arbiter、run/preflight failure 後の回復、転送 Reservation の一回消費を追加。Music では Talk 待機列に対する Service 経由の music 優先も追加した。provider/maxWorkers > 1 の入力でも同じサービス経路を通ることを試験設定に含めた。
- 検証: `mise -C E:\programming\AI_generative\fm-live-radio install`、`mise -C E:\programming\AI_generative\fm-live-radio x -- go test -count=1 ./internal/generation ./internal/musicgen ./internal/localtts`、同じ対象の `go test -race -count=1` がすべて exit 0。channel/barrier と deadlock検知用 timeout のみを使用し、新規 skip・実モデル・ネットワーク・GPU依存はない。
- 判断・引継ぎ: 追加テストは service 内の既存 preflight/runtime 注入点を利用し、production code・Player/app/cache/audio/docs requirement は変更していない。REQ-01〜03 の acceptance は未変更のまま、Player/App の同一契機、Shutdown、既存回帰、および第2段階は WP-2 以降に残る。

## WL-012 — Step 9: 第1段階統合受入 round 1 mandatory fix

- 日付・担当: 2026-09-12 / fresh fix worker。
- 対象: REQ-02/03、REQ-08。Acceptance列とleaderのfail判定は変更していない。
- 変更: `TestPrefetchNextStartsMusicBeforeTalk`をorder channelでMusic/Talkの実Generate entry順を検証する決定的テストへ変更。PlayerはMusic entry gateを導入し、Skip時はowner公開状態だけを無効化してin-flight countをjoinまで保持。Talk/Musicの実worker/job/context経路についてSkip/UpdateConfig/Shutdown、barrier cancel、join前busy、join後busy=0、旧結果のready/public item/history排除、replacement owner保護を検証するテストを追加した。Irodori duration fixtureはrepo内の計算済み独立期待値へ置換し、model asset依存を除去した。
- 検証: `mise -C <workspace> install`、指定6パッケージ通常/race、`mise -C <workspace> x -- go test -count=1 ./...`がすべてexit 0。新規テストはchannel/barrierのみでsleep・skip・外部モデル依存なし。
- 製品影響: 変更は同時契機のMusic→Talk entry順と、取消後のPlayer owner cleanupに限定。2曲先読み・genre/cacheは未着手。frontend/Wailsは製品/frontend差分なしのため再実行していない（既存round結果を保持）。

## WL-015 — Step 10: WP-3 中間受入 round 1

- 日付・担当: 2026-09-12 / persistent acceptance セッション、leader 記録。
- 対象: snapshot `e2d02ffdda4263b2a1240a807b9af0b45c0c5ae7`、diff SHA256 `8494F034B345A7BD7A9B5C0D7F9D3438206D2B95B67C5687158511411F832880`。
- 独立結果: mise install、対象通常/race、全Go testはexit 0。第1段階REQ-01/02はpass保持、REQ-03は補充失敗処理の再確認を付して保持。
- 判定: REQ-04〜07はfail。Talk pendingかつBGMなしの待機先、補充失敗の即時retry、Settings同値genreのfull resetに実装不一致。consume補充・同時hint・genre多段/選択競合・token/HTTP/TTL/複数保護のE1不足。
- 次: fresh fix workerへ期待/実際を限定して修正とT-05〜09の決定的試験を依頼し、同じacceptanceセッションで再受入する。

## WL-017 — Step 10: WP-3 中間受入 fix round 2

- 日付・担当: 2026-09-12 / persistent acceptance セッション、leader 記録。
- 対象: snapshot `066c6ea6e147f0ebe3db356cbddeb73408dfbca9`、diff SHA256 `49AEFBC91A63844122FA7E49C1C6C98E610DA5925615AA210677D28F582ABCC7`。
- 独立結果: mise install、対象通常/race、全Go testはexit 0。第1段階REQ-01〜03はpass保持。
- 改善確認: Talk pending+BGM空はMusic需要へ合流、Music失敗の即時retry停止、同値genre SaveConfigの状態維持を確認。
- 判定: REQ-04〜07はfail継続。T-05の実consume補充/同時hint/CacheLimit=1、T-06の全分岐、T-07/08の多段epochと登録競合、T-09のtoken/HTTP/TTL/複数保護が未検証。
- 次: fresh fix workerは不足E1の追加に限定し、テストが示す実装不一致だけを最小修正する。

## WL-019 — Step 10: WP-3 boundary round 3

- 日付・担当: 2026-09-12 / persistent acceptance セッション、leader 記録。
- 対象: snapshot `2bd0cce138d8cfc3e8d0dbffcaa4609e7d1763da`、diff SHA256 `B532CC105592DACB1F8D4540640AC87429698C66A80E332B00BBB1B9902A841D`。
- 独立結果: mise install、対象通常/race、全Go testがexit 0。REQ-01〜03はpass保持。
- 判定: REQ-04/05/07 pass。容量2・補充/失敗、Talk遅延全分岐、fallback provenance、複数保護とURL/TTL/Closeを確認。REQ-06は最終publish直前のepoch再確認欠落とA-B-C/A-B-A未検証でfail。
- 次: fresh fix workerがREQ-06の最終gateとmulti-epoch identityだけを修正・検証する。

## WL-021 — Step 10: WP-3 final fix round 4

- 日付・担当: 2026-09-12 / persistent acceptance セッション、leader 記録。
- 対象: snapshot `ff2cc71e24fd3c1c14123ac94520236ca2d6fa1b`、diff SHA256 `257511C0D94C5BE93D8B1B3D1D3564DC7DA27B3481450A5B2F64FF3A53E8AAE1`。
- 独立結果: mise install、対象通常/race、全Go、REQ-06 focused count=20がexit 0。
- 判定: REQ-06 pass。最終publish gateのepoch/genre再確認とURL解放・再選択、A-B-C/A-B-Aの旧epoch排除を確認。REQ-01〜05/07の既受入を保持し、WP-3全体pass。
- 次: WP-4でcurrent docs/as-built反映と最終mandatory checksを行い、final acceptanceへ移る。

## WL-023 — Step 9/11: WP-4回帰・as-built反映と最終受入開始

- 日付・担当: 2026-09-12 / WP-4 worker、leader 状態更新。
- 対象: snapshot `fba86d89`、full diff SHA256 `4f1469dabaea28f2ee63b9a28256a7b7838351050257dd3968e146d3304a4ec5`。
- 文書: current requirement/specification、generation-scheduling cheatsheet、index/linkを実装済み挙動へ更新。旧契約は調査当時の記録として明示した箇所だけに保持。
- 実装側検証: C-01〜04すべてexit 0、ローカルMarkdownリンク到達。新規制御testのskipなし。C-04の既知manifest linker出力と12GB実機/E2E対象外を明記。
- 状態: 全REQが証拠付きimplemented、全WP統合済みのため `implementing` から `accepting` へ遷移。persistent acceptanceセッションへ最終受入をを依頼する。

## WL-024 — Steps 10–11: 最終独立受入・完了

- 日付・担当: 2026-09-12 / persistent acceptance セッション、leader 完了更新。
- 受入対象: snapshot `df0e15b40da334a24f9af40e40f088f3ae3a47e7`、parent `6265717e`、diff SHA256 `D97CE51DFC341140871B9B73C076345447632977CB7ABFAE54286C654FD3E0BD`。
- 独立E1: C-01全Go、C-02対象race、C-03 frontend build、C-04 Wails buildがすべてexit 0。新規制御testのskipなし。C-04の既知manifest linker出力はbaselineと同じでEXE生成済み。
- E3/範囲: REQ-01〜08のコード・test・current requirement/specification/cheatsheet一致、ローカルリンク、依存差分なし、不要artifactなしを確認。
- 判定: 全REQ pass、mandatory fix/gap/人間確認待ちなし。E2E、12GB実機性能、実GPU、実UIは合意済み対象外として未実施。
- 状態: Completion Gateを満たしたため `accepting` から `done` へ更新し、plan indexを同期。

## WL-013 — Step 10: 第1段階統合受入 fix round 2

- 日付・担当: 2026-09-12 / persistent acceptance セッション、leader 記録。
- 対象: snapshot `9d0f88ca0399b1270fa4f43d4d27573a4b9a5a28`、diff SHA256 `ADCF6E6FED26BC94E44FFF733114095DEC84BFD9369C322FB93574F3453EA321`。
- 独立結果: `mise -C <workspace> install`、C-01全Go、対象raceがexit 0。前roundのfrontend/Wails buildはfix差分の影響外として保持。
- 判定: REQ-01〜03の第1段階をpass。同一契機Music→Talk順、Talk/Music双方のactual workerでSkip/UpdateConfig/Shutdownのcancel→join、busy維持/解除、旧結果非公開、replacement owner保護を確認。外部parity fixture依存は同じ契約を検査する自己完結testへ修正。
- 次: mandatory fixなし。WP-3の2曲先読み、Talk遅延時継続、ジャンル世代、fallback provenance、WAV参照保護へ進む。

## WL-014 — Step 9: WP-3 実装結果

- 日付・担当: 2026-09-12 / WP-3 fresh implementation worker。
- 対象: REQ-04〜07。`internal/player/player.go` に未再生 BGM FIFO（ready＋逐次補充）、Talk 未完成時の BGM 継続、音楽専用 `musicEpoch` と最終公開ゲートを実装し、genre-only の `UpdateConfig`/`UpdateStableAudio3Genre` は Talk・現在 item・履歴を維持して音楽だけを切り替える経路へ統合した。`app.SaveConfig` は genre を正規化して Player 経路へ渡す。
- ファイル保護: `internal/musicgen` に生成時 genre sidecar の atomic 保存、sidecar provenance fallback、複数 protected path を飛ばす TrimCache と WAV/sidecar 同時整理を追加。`internal/fileprotect` と `internal/audio/server.go` に token の TTL/Close/明示解放、HTTP 読み取り中の参照保護を追加した。
- テスト: `TestMusicPrefetchFillsTwoFIFOAndRefillsAfterConsume`、`TestMusicGenreChangeRejectsLateOldEpochResult`、`TestPickFallbackForGenreRequiresValidMatchingSidecar`、`TestTrimCacheSkipsProtectedWAVAndRemovesSidecarTogether` を channel/barrier と TempDir で追加。`go test -count=1 ./internal/player ./internal/musicgen ./internal/audio`、同対象 `go test -race -count=1 ...`、全 Go `go test -count=1 ./...` は Go 1.26.1 実体で exit 0。mise shim は通常権限では起動拒否だったため、`mise which go` で解決した同一ツールチェーンの実体を使用した。
- 引継ぎ: 実装状態・証拠のみ更新し、Acceptance欄・REQ-01〜03/08・plan文は変更していない。frontend/Wails と current docs は WP-4/最終受入へ残す。

## WL-020 — Step 9: WP-3 REQ-06 最終 publish gate / multi-epoch fix

- 日付・担当: 2026-09-12 / fresh fix worker。
- 対象: REQ-06 の mandatory fix 2点のみ。Acceptance欄・REQ-01〜05/07・plan文は変更していない。
- 変更: `internal/player/player.go` の `pickBGM` を選択 context と最大試行回数を持つ loop へ変更。RegisterFile 後の初回 epoch/genre check 通過後に unexported `beforeBGMFinalPublish` hook を置き、最終 publish gate で generation/closed、`musicEpoch`、result/current normalized genre を同一 `p.mu` 下で再確認する。不一致時は登録 URL を解放して最新 epoch の需要へ戻す。`internal/player/player_test.go` に hook barrier の旧 URL release・新 genre item・旧 ready/status 非混入試験と、context reservation を Acquire して lease を barrier 解放後に Release する A→B→C / A→B→A identity 試験を追加した。
- 検証: `mise -C <workspace> install`、`mise x -- go test ./internal/player -count=1`、`mise x -- go test -race ./internal/player -count=1`、関連 generation/player/musicgen/audio/fileprotect の通常/race、`mise x -- go test ./... -count=1` がすべて exit 0。REQ-06 新規試験は `-count=20` でも exit 0。sleep 依存・frontend/Wails API変更なし。
- 引継ぎ: REQ-06 実装状態を `implemented` とし evidence のみ更新。Acceptance欄の `fail` は独立受入セッションが再検証して判断する。

## WL-022 — Step 9相当: WP-4 最終回帰

- 日付・担当: 2026-09-12 / WP-4 文書・回帰 worker。
- 目的: REQ-01〜07の独立受入済み実装を最新ツリーで回帰し、REQ-08の実装証拠を揃える。Acceptance欄と計画stateは変更しない。
- 初期化: `mise install` を最初に試行したが、WinGet shim（`mise.exe` 0 byte）の起動に失敗。承認済み昇格実行で同じmise実体の `mise install` を完了（Go 1.26.1 / Node 24.14.1 / Bun は既導入）。
- 結果: C01 `mise -C <workspace> x -- go test -count=1 ./...`、C02 `mise -C <workspace> x -- go test -race -count=1 ./internal/generation ./internal/player ./internal/musicgen ./internal/localtts ./internal/talk ./internal/audio ./internal/fileprotect`、C03 `mise -C <workspace> x -- npm --prefix frontend run build`、C04 `mise -C <workspace> run build` は全て exit 0。C04は `.rsrc merge failure: multiple non-default manifests` を linker が出力したが、Wails/EXE生成は完了。
- skip/warning: 新規の調停・FIFO・epoch・cache・token検証に skipなし。既存の実モデル/GPU opt-in test は環境変数未指定のためskip（`FM_RADIO_IRODORI_MODEL`/`FM_RADIO_IRODORI_REF`、`FM_RADIO_GPU_COLLECTOR_E2E`/`FM_RADIO_GPU_SAMPLER_E2E`）。12GB性能・実GPU E2Eの判定には使用しない。

## WL-023 — Step 11相当: WP-4 as-built 文書フィードバック

- 日付・担当: 2026-09-12 / WP-4 文書 worker。
- 反映: [current requirements](../requirement.md) と [current specification](../specification.md) の最終確認日を2026-09-12へ更新し、共有ArbiterのRuntime load〜Close排他、Music優先/FIFO、同一需要共有、未再生2曲、Talk遅延時BGM、music epoch/genre最終gate、sidecar fallback、CacheLimitとの分離、token/file保護、cancel→join→Closeを確認済みas-builtとして追記した。
- 反映: [generation-scheduling cheatsheet](../cheatsheet/generation-scheduling.md) は計画前の現行事実と実装後as-builtを章分けし、確定API、C01〜C04、既知制約、opt-in skip理由を追記。[cheatsheet index](../cheatsheet/index.md) と [cheatsheet links](../cheatsheet/link.md) の参照先・説明を同期した。
- 境界: `90_status.md` のAcceptance列と計画stateは変更せず、REQ-08の実装状態/evidenceだけを更新した。未確認の12GB実機性能改善、実GPU E2E成功、実UI起動結果はas-builtへ昇格していない。
