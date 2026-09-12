# Current Specification

最終確認日: 2026-09-12

この文書は、現在実装されている `fm-live-radio` の実装仕様を示す。現行コードと一致する構造、データ、フロー、環境制約のみを記載する。

## 2026-09-08 as-built（WP-5実装）

v4.1は `manifest.json` のschema/graph/tokenizer/external dataをTalk開始前に検査し、1 Talk内でRuntimeを再利用する。製品bundleへのparity fixture同梱・fixture hashは必須ではなく、fixture hashはparity検証時だけ記録・比較する。新規設定のIrodori modelDirはv4.1で、保存済みv3・任意パスは保持する。`cmd/tts-benchmark` は製品Service/Talk経路で固定10原稿（200–300字）のpreflight/load/各文/結合/close、RTF、期限、推論中pollしたnvidia-smi process VRAM（WDDMでprocess値がN/Aの場合は厳密PID/DXGI LUID/GPU UUID照合のWindows CIM `DedicatedUsage`へfallbackし、device-totalは正式process値へ代入しない）、20回定常反復、p95 v4/v3比とWAV manifestをJSON/CSVへ保存する。`cmd/tts-e2e` はRSS/LLMだけloopback fixtureを使い、実Stable Audio BGM・製品Player・Talk・audio server/loudness、v4/v3子プロセス再起動、preflight負例、Skip/取消join/close/active shutdownを実行する。Stable Audioやv4資産が無い場合はfixtureへフォールバックせず非zeroで報告する。CPU/CUDA/autoはORT共有のため別プロセスで実行する。

## 2026-09-12 as-built（生成リソース制御 / WP-1〜3）

`internal/generation.Arbiter` はプロセス共有・容量1の予約調停器であり、`musicgen.Service.Generate` と `localtts.Service` の Runtime ロード直前から推論完了後の `Close` 完了までを覆う。待機時は Music を優先し、同種は FIFO とする。`generation.Reservation` は `generation.WithReservation` で Player からサービスへ一度だけ転送できる。待機中の取消・期限切れはロードせず、許可後の取消は実 worker の join と Runtime close 後に `Lease.Release` する。

`Player` は同期取得と先読みを Talk/BGM の需要 job に統合し、同じ需要を二重生成しない。`PrefetchNext` は Music reservation を同期登録してから Talk worker を登録する。同時待機では Music の entry gate を先に通す。`musicReady` FIFO は未再生 ready 曲と予約を合わせて最大2件とし、最初の1件を返却後、不足分だけを逐次補充する。補充失敗は同じ境界で再試行せず、明示的な hint または消費が次の retry boundary になる。

Talk slot で Talk が未完成なら、`NextItem` はその Talk job を継続し、ready BGM があれば返却する。BGM も空の場合は同じ Music job に合流して待つ。Talk slot・履歴は Talk を実際に採用するまで消費せず、Talk 失敗時だけ slot を一度消費して BGM fallback へ進む。

ジャンル変更は `musicEpoch` だけを進め、music FIFO と旧 music job を無効化する。現在再生中の item、Talk job、記事履歴は維持する。生成結果は snapshot の epoch/owner と正規化済み genre を照合し、`RegisterFile` 後の最終公開直前にも `generation`、`closed`、epoch、genre を同一 mutex 下で再確認する。不一致なら登録済み URL を解放して最新設定で選び直すため、A→B→C / A→B→A の遅延結果は公開されない。同値の genre 保存は音楽以外の状態をリセットしない。

`musicgen.Service` は生成時 genre を WAV sidecar（`<wav>.json`）へ atomic 保存し、`PickFallbackForGenre` は正規化済み genre と一致する有効 sidecar の WAV だけを候補にする。`TrimCache` は `fileprotect` の複数保護 path とその sidecar を維持し、保護数が上限を超える間は削除を延期する。`CacheLimit` はディスク保存件数であり、Player の未再生2曲枠とは独立する。

`audio.Server.RegisterFile` は token の TTL と共有参照を登録し、HTTP の `/audio/<token>` 読み取り中は request-scoped reference を保持する。`ReleaseAudioURL`、TTL失効、`Close` は対応 token/envelope と保護を解放する。WAV envelope の precompute に失敗しても audio URL 登録は成功し、`/loudness` は 204、未知/期限切れ token は 404 を返す。

## 技術スタック

- Desktop shell: Wails v2
- Backend: Go
- Frontend: React 18 + TypeScript + Vite
- Local inference: `github.com/yalue/onnxruntime_go`
- RSS parsing: `github.com/mmcdole/gofeed`
- HTML article extraction: `github.com/PuerkitoBio/goquery`
- Toolchain: `mise`

## エントリポイント

- `main.go`: Wails app を起動する。
- `app.go`: Wails binding とアプリ lifecycle を持つ。
- `frontend/src/App.tsx`: メイン UI と Wails API 呼び出しを持つ。

`App.startup` は以下を初期化する。

1. `store.New()`
2. `LoadConfig()`
3. `LoadHistory()`
4. `audio.Start()`
5. `temp_audio/` 作成と起動時 cleanup
6. `talk.New(tempDir)`
7. `musicgen.New()`
8. `player.New(cfg)`

`App.shutdown` は `player.Shutdown()` で Player 所有の worker と先読みを cancel→join し、audio server の token を Close で解放した後、`generation.Shutdown()` で ONNX Runtime environment を破棄する。

## Wails API

`app.go` がフロントエンドへ公開する API:

- `LoadConfig() (domain.AppConfig, error)`
- `SaveConfig(cfg domain.AppConfig) error`
- `GetNextItem(req domain.NextItemRequest) (domain.PlayableItem, error)`
- `SkipCurrent(req domain.SkipRequest) (domain.PlayableItem, error)`
- `GetStatus() (domain.AppStatus, error)`
- `PrefetchTalk()`

`SaveConfig` は config を保存し、既存 `player` に `UpdateConfig` を反映する。`GetNextItem` と `SkipCurrent` は `player` から返された履歴更新がある場合、`history.json` に保存する。

`SaveConfig` は Stable Audio 3 genre を正規化してから保存する。genre だけが変わる保存は `Player.UpdateConfigFromSave` の music-only 経路を通り、music epoch/FIFO/予約だけを更新する。同値の正規化結果では Talk、現在 item、履歴、再生カウンタをリセットしない。`UpdateStableAudio3Genre` も同じ music-only 経路を使う。

## データモデル

主要な型は `internal/domain/types.go` に定義される。

### Source enum

- `PlayableKind`
  - `bgm`
  - `talk`
  - `silence`

### AppConfig

`AppConfig` は以下の設定群を持つ。

- 基本設定:
  - `rssUrls`
  - `bgmVolume`
  - `talkVolume`
- `TalkConfig`:
  - `enabled`
  - `cycleBgmCount`
  - `targetDurationSec`
  - `silenceGapMinMs`
  - `silenceGapMaxMs`
- `LLMConfig`:
  - `enabled`
  - `baseUrl`
  - `apiKey`
  - `model`
- `LocalInferenceConfig`:
  - `ortLibraryPath`
  - `maxWorkers`
  - `executionProvider`
  - `deviceId`
- `StableAudio3Config`:
  - `modelDir`
  - `outputDir`
  - `promptBase`
  - `genre`
  - `seconds`
  - `steps`
  - `seedMode`
  - `fixedSeed`
  - `cacheLimit`
- `IrodoriConfig`:
  - `modelDir`
  - `narratorDir`
  - `refWav`
  - `seconds`
  - `numSteps`
  - `seedMode`
  - `fixedSeed`
  - `cfgText`
  - `cfgCaption`
  - `cfgSpeaker`
  - `durationScale`

### PlayableItem

`PlayableItem` は UI が再生する 1 item を表す。

- `id`
- `kind`
- `url`
- `loudnessUrl`
- `mime`
- `title`
- `artist`
- `topicTitle`
- `durationHintMs`
- `source`

`silence` の場合は `url` を持たず、`durationHintMs` に基づいてフロントエンド側の timer で待機する。`loudnessUrl` は audio server の `/loudness/<token>` を指し、対象が 16-bit PCM WAV で envelope precompute に成功した場合のみ意味のあるレスポンスを返す。それ以外（非 WAV / decode 失敗 / token 期限切れ）は server 側で 204 / 404 となり、フロントエンドは合成アニメーションへ fallback する。

### AppStatus

`AppStatus` は UI indicator 用の軽量状態である。

- `talkPrefetching`
- `talkReady`
- `musicGenerating`
- `musicReady`
- `localGenerationError`

## 永続化

`internal/store` が OS user config directory 配下に `fm-live-radio` ディレクトリを作成する。

- `config.json`: `AppConfig`
- `history.json`: `History`
- `temp_audio/`: Talk WAV などの一時音声

`SaveConfig` と `SaveHistory` は一時ファイルへ書き出してから rename する atomic write を使う。`config.json` は `0600` permission で保存される。

`History.UsedArticleUrls` は最大 500 件に trim される。

## 既定値

`store.DefaultConfig()` の主要値:

- `<base>`: アプリ起動時の current working directory
- `bgmVolume`: `0.8`
- `talkVolume`: `1.0`
- `talk.enabled`: `true`
- `talk.cycleBgmCount`: `3`
- `talk.targetDurationSec`: `60`
- `talk.silenceGapMinMs`: `1000`
- `talk.silenceGapMaxMs`: `3000`
- `llm.enabled`: `true`
- `llm.baseUrl`: `http://localhost:11434/v1`
- `llm.model`: `gpt-4o-mini`
- `localInference.maxWorkers`: `1`
- `localInference.executionProvider`: `auto`
- `localInference.deviceId`: `0`
- `stableAudio3.modelDir`: `<base>/model/sa3-sm-music`
- `stableAudio3.outputDir`: `<base>/generate_music`
- `stableAudio3.promptBase`: `instrumental background music for a radio show, seamless loop feel, no vocals`
- `stableAudio3.genre`: `chill lo-fi`（許可値: `chill lo-fi`, `smooth jazz`, `minimal electronica`, `ambient music`）
- `stableAudio3.seconds`: `30`
- `stableAudio3.steps`: `8`
- `stableAudio3.seedMode`: `random`
- `stableAudio3.cacheLimit`: `20`
- `irodori.modelDir`: `<base>/model/irodori-v4.1`
- `irodori.narratorDir`: `<base>/narrator`
- `irodori.seconds`: `-1`
- `irodori.numSteps`: `40`
- `irodori.seedMode`: `random`
- `irodori.cfgText`: `3`
- `irodori.cfgCaption`: `3`
- `irodori.cfgSpeaker`: `5`
- `irodori.durationScale`: `1`

`applyConfigDefaults` は古い config で欠落した値を補完し、音量を `[0..1]` に clamp する。未知の execution provider は `cpu` に正規化される。`stableAudio3.genre` は空文字・未対応値ともに `chill lo-fi` へ正規化される（`store.NormalizeStableAudio3Genre`）。

## 再生フロー

`internal/player.Player` が再生順序を管理する。

1. `GetNextItem` が `Player.NextItem` を呼ぶ。
2. `pendingSilence` が true の場合、まず `silence` item を返す。
3. `bgmCountSinceLastTalk >= talk.cycleBgmCount` なら Talk slot と判断する。
4. ready Talk があれば consume して `talk` item を返す。
5. Talk slot で Talk job が未完成なら、その job を維持したまま ready BGM を返す。BGM が空なら同じ Music job を待つ。
6. Talk job がなければ同期取得と既存先読みを同じ job に合流する。Talk 生成失敗時は slot を一度消費し、BGM fallback または生成エラーを返す。
7. BGM 選択は `musicReady` FIFO の先頭を消費し、空なら同じ Music job を待つ。登録・公開直前に epoch/genre/owner を再確認する。
8. BGM 消費後は未再生 ready＋予約が2件になるまで不足分だけ Music を補充する。Talk slot が近い同一契機では Music reservation を先に登録してから Talk prefetch を開始する。

`Skip` の動作:

- BGM skip は BGM count を進める。
- Talk skip は Talk slot を消費し、ready Talk を破棄する。
- Silence skip は無音を消費する。
- in-flight の Talk / Music prefetch は cancel されるが、実 worker の join と Runtime close 完了まで稼働数を保持する。

## Audio server

`internal/audio.Server` は `127.0.0.1:0` で起動し、動的 port の local HTTP server として動作する。

- path は `/audio/<token>` および `/loudness/<token>`。
- `RegisterFile(path, ttl)` は file path を token と TTL に紐づける。
- token は UUID で生成される。
- expired token は request 時と 30 秒ごとの GC loop で削除される。expired 時には対応する envelope cache も削除される。
- token 登録時と HTTP `/audio/<token>` 読み取り中は `fileprotect` の参照を保持する。`ReleaseAudioURL`、TTL失効、`Close` は token と envelope を削除し、対応する参照を解放する。読み取り中の request-scoped reference が残るため、`TrimCache` は配信中の WAV を削除しない。
- MIME type は file extension から best-effort で設定される。
- `RegisterFile` は対象 file 拡張子が `.wav` の場合のみ、16-bit PCM WAV と仮定して loudness envelope を計算してメモリ上にキャッシュする（`audiofmt.ComputeWavLoudnessEnvelopeFile`、window 50 ms）。decode 失敗・非 WAV は log warning のみとし、`RegisterFile` 自体は成功させる。
- `/loudness/<token>` は JSON response `{windowMs, sampleRate, durationSec, rms, peak?}` を返す。値はすべて `[0, 1]` に clamp 済みの正規化値（`abs(sample) / 32768`）。
  - token 未知 / 期限切れ: `404 Not Found`。
  - envelope cache 未生成（非 WAV / decode 失敗など）: `204 No Content`。
  - 成功時に `Access-Control-Allow-Origin: *`、`Access-Control-Allow-Methods: GET, OPTIONS`、`Access-Control-Allow-Headers: *` を付与し、`OPTIONS` preflight を 204 で許可する。
- `Server.LoudnessURLForAudioURL(audioURL)` は `RegisterFile` が返した audio URL から対応する `/loudness/<token>` URL を導出するヘルパー。`player` から `PlayableItem.LoudnessURL` の設定に使う。
- `audio.Server` の URL 参照保護は Player FIFO の生成結果受渡しと組み合わせて使う。`musicgen.ProtectResult` が FIFO/選択中の WAV を保持し、URL 登録後は server token の参照へ所有権を移す。

## BGM 実装

### Stable Audio 3

`internal/musicgen.Service` が Stable Audio 3 生成を扱う。

- `Generate(ctx, cfg)`:
  - model dir と output dir を検証する。
  - execution provider を `generation.ConfigureExecutionProvider` に反映する。
  - ONNX Runtime を `generation.Init` で初期化する。
  - output dir を作成する。
  - prompt と seed を解決する。
  - `music_<unixnano>.wav` に出力する。
  - `stableaudio/pipeline` の runtime を初期化して `Synthesize` を実行する。
  - 成功後に cache trimming を行う。
  - 戻り値 `Result` には `Genre`（正規化済み）と `Prompt` を含める。
- `Generate` は `generation.Arbiter` に Music 予約を登録し、Runtime の load 前から `Close` 完了まで lease を保持する。ctx から同種の未消費予約を受け取った場合はそれを一度だけ消費する。
- `Fallback(cfg)`:
  - output dir から現在の正規化済み genre と sidecar provenance が一致する fallback WAV を選ぶ。sidecar がない、壊れている、または genre が不一致の WAV は候補にしない。
  - 戻り値 `Result` には `Genre`（正規化済み）を含める。

seed 解決:

- `fixed`: `fixedSeed`
- `sequential`: current Unix time
- その他: random uint32

#### ジャンル (genre)

- 許可値は固定の 4 つのみ: `chill lo-fi`, `smooth jazz`, `minimal electronica`, `ambient music`。
- 既定値は `chill lo-fi`。
- `BuildPrompt` は `cfg.StableAudio3.Genre` を直接使わず、`store.NormalizeStableAudio3Genre` で正規化した値（`SelectedGenre`）を `GenrePromptDescription` で説明文へ展開する。
- prompt 構築順: `GenrePromptDescription(SelectedGenre), promptBase, instrumental, background music, no vocals` を `, ` で結合。
- genre descriptor は config には保存しない。`config.json` は短い genre 名だけを保持する。
- descriptor の概要:
  - `chill lo-fi`: lo-fi hip hop texture、dusty drums、mellow keys、vinyl noise、warm tape saturation、late-night mood。
  - `smooth jazz`: smooth jazz ensemble feel、warm electric piano、clean guitar or sax-like lead、brushed drums、relaxed sophisticated groove。
  - `minimal electronica`: minimal electronic composition、sparse synth patterns、precise soft pulses、restrained bass、clean modern atmosphere。
  - `ambient music`: ambient soundscape、slow evolving pads、airy textures、no strong beat、spacious calm immersive atmosphere。
- `playable.Source` には `genre` と `prompt` を含める。
- ジャンル更新時の非中断挙動: `App.UpdateStableAudio3Genre` または genre-only の `App.SaveConfig` は `player.UpdateStableAudio3Genre` / `UpdateConfigFromSave` を呼び、Talk/current item/history を維持する。`musicEpoch` を進めて music FIFO と旧音楽予約だけを無効化し、変更後に選択確定する次の BGM へ反映する。同値の正規化結果では epoch を進めない。

## Talk 実装

`internal/talk.Service` が Talk 生成を扱う。

`localtts.Service` は `generation.Arbiter` の Talk 予約を Runtime load 直前から `Close` 完了まで保持する。RSS/LLM の準備、文単位の合成、結合を同一 Talk の処理として扱い、cancel 後も実 worker の join と close が完了してから lease を解放する。

1. `Talk.Enabled` と RSS URL の有無を確認する。
2. `rss.Picker.Pick` で未使用 article を選ぶ。
3. `llm.OpenAICompat.Complete` で Talk 原稿を作る。
4. `localtts.Service` (IrodoriTTS) で音声合成を行う。
5. 一時ファイルとして `temp_audio/talk_YYYYMMDD_HHMMSS.wav` に保存する。

system prompt は、落ち着いたラジオ DJ としてニュースを 1 分で紹介する日本語口語原稿を要求する。user prompt は article title、feed title、本文を含む。本文は最大 2000 rune に制限される。

## RSS 実装

`internal/rss.Picker` の主要仕様:

- HTTP timeout: 10 秒
- 最大試行 feed 数: 5
- feed ごとの最大 item 数: 30
- 有用本文の指示閾値: 120 rune
- RSS item の `Content` が空なら `Description` を使う。
- 本文が短い場合、article URL の HTML を取得し selector 抽出を試みる。

汎用 selector:

- `article p`
- `article li`
- `main p`
- `main li`
- `.article-body p`
- `.article__body p`
- `.entry-content p`
- `.post-content p`
- `#article p`

一部の Impress 系 host には専用 selector がある。

## LLM 実装

`internal/llm.OpenAICompat` は OpenAI 互換 Chat Completions API を呼ぶ。

- endpoint: `<baseUrl>/chat/completions`
- method: `POST`
- request:
  - `model`
  - `messages`
  - `temperature`: `0.6`
  - `max_tokens`: `8192`
- `apiKey` が空でなければ `Authorization: Bearer <apiKey>` を付ける。
- default HTTP timeout: 120 秒
- non-2xx response は `llm http error` として扱う。

## IrodoriTTS 実装

`internal/localtts.Service` が IrodoriTTS を扱う。

- `SynthesizeWav(ctx, cfg, text)`:
  - model dir を検証する。
  - execution provider を設定する。
  - ONNX Runtime を初期化する。
  - model assets を検証する。
  - mutex で同時合成を 1 本に制限する。
  - 文単位に分割して合成する。

model asset 検証:

- `tokenizer.json` が存在する。
- metadata exports に記載された file path が存在する。

参照 WAV 解決:

1. `irodori.refWav` が空でなければ使う。
2. `irodori.narratorDir` の先頭の `.wav` を使う。
3. 見つからなければ空文字列を返す。

出力仕様:

- sample rate: 48 kHz
- channels: mono
- PCM: 16-bit
- 文間 gap: 300 ms
- 文単位合成失敗時の代替無音: 3 秒

## ONNX Runtime 実装

`internal/generation` が ONNX Runtime の shared library と execution provider を管理する。

DLL path 解決順:

1. `localInference.ortLibraryPath`
2. `FM_RADIO_ORT_LIB`
3. `IRODORI_ORT_LIB`
4. `SA3_ORT_LIB`
5. `third_party/onnxruntime-gpu/onnxruntime-win-x64-gpu-1.26.0/lib/onnxruntime.dll`
6. `third_party/onnxruntime/onnxruntime-win-x64-1.26.0/lib/onnxruntime.dll`
7. `onnxruntime.dll`

execution provider 解決:

- `FM_RADIO_ORT_EP` があれば config より優先する。
- `FM_RADIO_ORT_DEVICE_ID` が parse できれば device ID として使う。
- provider は `auto`、`cuda`、`cpu` に正規化する。
- 不明 provider は `cpu` として扱う。
- negative device ID は `0` に丸める。

session option:

- `cpu`: provider option なし。
- `cuda`: CUDA provider option を作成して append する。
- `auto`: CUDA provider option 作成に成功すれば CUDA、失敗すれば warning を記録して CPU。

制約:

- `ort.InitializeEnvironment()` は process 内で一度だけ実行される。
- 初期化後に異なる DLL path または provider を指定した場合は error を返す。
- DLL directory は `PATH` に追加される。

## フロントエンド仕様

`frontend/src/App.tsx` は Wails generated API を呼び出す。UI は木目調のラジオ筐体、金属パネル、アンバーのチューニングメーター、LED 状態表示、波形窓で構成される。

ファイル:

- `frontend/src/App.tsx`: 画面構成、Wails API 呼び出し、再生制御。
- `frontend/src/Visualizer.tsx`: 常時オンエアの波形ビジュアライザ (Canvas + requestAnimationFrame)。
- `frontend/src/style.css`: ラジオ風テーマのデザイントークンと全体スタイル。
- `frontend/src/App.css`: ラジオ筐体、操作盤、波形窓、ノブ、チューニングメーター、Settings モーダルのレイアウト。

Wails window:

- `main.go` の `options.App` は `Width: 1280`, `Height: 860`, `MinWidth: 900`, `MinHeight: 680` を指定する。

画面構成:

- Header:
  - brand
  - 右上 Settings button
- Control panel:
  - 黒ガラス風の波形窓
  - ON AIR / OFF AIR インジケータ
  - Visualizer (波形)
  - progress bar / elapsed / duration
  - Play / Pause button (発光する円形ボタン)
  - Skip button
  - BGM volume knob
  - Talk volume knob
  - Talk / Music / Local error LED
- Tuning panel:
  - ON AIR / OFF AIR インジケータ
  - current kind
  - title / subtitle
  - SA3 Genre tuner: 4 つの固定値（`chill lo-fi`, `smooth jazz`, `minimal electronica`, `ambient music`）をボタンとして表示。選択時は `App.UpdateStableAudio3Genre` を呼び、現在再生中・prefetch 中の BGM を中断せず、次回 BGM 生成から反映する。
- Nameplate:
  - 現在のタイトルを銘板風に表示する。
- Settings modal:
  - アプリ機能設定（曲数、Silence Gap、BGM/Talk音量、RSS、LLM）
  - 生成設定（details タグで初期非表示。ORT、Stable Audio 3、Irodori）
    - SA3 Genre: Console と同じ 4 値を `AppConfig.stableAudio3.genre` に保持し、Settings の Save で `App.SaveConfig` 経由で永続化。

Visualizer:

- `playing` / `kind` / `level`(現在 kind の音量) / `audio`(現在の `<audio>` 要素) / `loudness`(現在 item の precomputed envelope or null) を入力に、振幅・速度・色相を補間してなめらかに変化させる。
- アイドル/一時停止でも静かに流れ続け、「音が流れ続ける」コンセプトを表現する。
- 実音声の FFT 解析、`AnalyserNode`、`captureStream`、`AudioContext` は使わない。バックエンドが事前計算した RMS envelope を 50 ms 窓で参照する方式を採る。
- 描画 frame ごとに、`<audio>.currentTime` を `envelope.windowMs` で割って RMS 値を取得し、`level` を乗算したうえで kind/level ベースの `amp` / `energy` 目標値に混ぜる（amp に `+raw * level * 0.55`、energy に `+raw * level * 0.35`、いずれも clamp）。
- `playing=false`、`kind='silence'`、`audio.paused`、envelope 不在のいずれかの場合は loudness の混入を行わず、既存の合成アニメーションへ静かに fallback する。
- `prefers-reduced-motion` 時は静止フレームを描画し、状態変化時のみ再描画する。

App.tsx の loudness fetch:

- `current` 変更時に envelope state を即座に `null` へリセットする。
- `current.loudnessUrl` が存在し、`current.kind !== 'silence'` の場合のみ `AbortController` 付きで `fetch` する。
- 古い fetch が item 切替・skip 後に解決した場合は、`currentIdRef.current !== itemId` の比較で破棄する。`AbortController.abort()` も併用して通信自体を打ち切る。
- 失敗（network / non-2xx / 204 / parse error）は toast 表示せず、合成アニメーション fallback のままにする。
- 受信した `rms` / `peak` 配列は `[0, 1]` に再 clamp する。`windowMs` が数値かつ正、`rms` が空でない配列のときのみ採用する。
- `stopPlayback` 時にも envelope を `null` へリセットする。

再生:

- `GetNextItem` で次 item を取得する。
- `silence` は browser timer で duration を消費する。
- `bgm` / `talk` は returned URL を `<audio>` に設定して再生する。
- `<audio>` の `onEnded` で次 item へ進む。
- `<audio>` の `onError` でも次 item を試す。
- BGM 再生中の subtitle は `BGM · stable_audio_3 · <genre>` 形式で `source.genre` を含める。provider または genre が欠落している場合は存在する項目のみを ` · ` 区切りで表示する。
- 再生中は 500 ms 間隔で `GetStatus` を poll する。
- 再生 progress は 250 ms 間隔で更新する。
- 980px 以下では操作パネルとチューニングパネルを縦積みにし、720px 以下では操作盤内の transport / mixer / lamp も縦積みにする。

## 開発・検証

標準コマンド:

```powershell
mise install
mise run setup
mise x -- go test ./...
mise x -- npm --prefix frontend run build
mise run build
```

明示v4.1サービス smoke test:

```powershell
mise run tts-smoke
```

CUDA 強制 smoke test:

```powershell
$env:FM_RADIO_ORT_EP='cuda'
mise x -- go run ./cmd/tts-smoke --service --model model/irodori-v4.1 --ep cuda --steps 2 --seconds 0.5
Remove-Item Env:FM_RADIO_ORT_EP
```

`cmd/tts-smoke` はIrodori v4.1のpreflight、生成WAV、Runtime lifecycleを確認する。`mise run tts-e2e` はlocal RSS/LLM fixtureの3周期、`mise run tts-benchmark` は固定10原稿・20回定常反復を実行する。CPU/CUDA/autoは共有ORTのため別プロセスで実行する。

## Tokenizer互換性（2026-09-08）

internal/localtts/irodori/tokenizerは固定Irodori v4.1 tokenizerの設定、特殊token、UTF-8 byte fallback、padding/truncationを解釈し、旧v3処理をlegacy分岐で保持する。EncodePaddedCheckedは非正長をエラーにする追加API。既存pipelineのtext256/caption64と呼出しAPIは維持する。固定公式との656条件一致、v3旧実装との656条件一致を[独立検証](plan_20260907_irodori_v4/evidence/wp2-acceptance.md)した。これはtokenizer対応のみで、製品のv4推論対応や既定モデル変更は含まない。

## v4.1の現在の受入状況（2026-09-08）

利用者の採用承認に基づき、新規設定の既定をv4.1へ変更した。保存済みv3・任意パスと参照音声設定は保持する。Runtime再利用は1 Talk内に限定し、取消時は推論完了/join後にCloseする。Player予約のcleanupは世代と所有者が一致する場合だけ行う。常駐Runtime cacheは未採用。

正式CUDA40step/自動durationの製品E2E3周期、CPU/auto fallback、全Go/race/frontend/Wails build、実Settings保持を独立確認した。旧比較基準ではp95比1.93365と定常VRAM増加幅が未達だったが、利用者承認により資源同等性を採用条件から外した。元の測定値・欠測は診断情報として保持する。正式60 WAVの生成証拠を保持し、聞き取れるアナウンス品質は利用者が受け入れた。全ペア個別採点済みとは扱わない。実UIは利用者が起動・一通りの動作確認を行い、問題なしとして現状を最終承認した。個々の操作ログの提出とは区別する。benchmarkは件数・条件・全phase生成成功・v4期限・child失敗を採用判定に反映し、p95比とVRAM比較を別の診断欄へ記録する。正本は[計画status](plan_20260907_irodori_v4/90_status.md)、測定境界は[TTS検証手順](cheatsheet/tts-benchmark-e2e.md)。



