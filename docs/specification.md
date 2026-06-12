# Current Specification

最終確認日: 2026-06-13

この文書は、現在実装されている `fm-live-radio` の実装仕様を示す。現行コードと一致する構造、データ、フロー、環境制約のみを記載する。

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

`App.shutdown` は audio server を停止し、`generation.Shutdown()` で ONNX Runtime environment を破棄する。

## Wails API

`app.go` がフロントエンドへ公開する API:

- `LoadConfig() (domain.AppConfig, error)`
- `SaveConfig(cfg domain.AppConfig) error`
- `ScanGenres() ([]string, error)`
- `GetNextItem(req domain.NextItemRequest) (domain.PlayableItem, error)`
- `SkipCurrent(req domain.SkipRequest) (domain.PlayableItem, error)`
- `GetStatus() (domain.AppStatus, error)`
- `PrefetchTalk()`

`SaveConfig` は config を保存し、既存 `player` に `UpdateConfig` を反映する。`GetNextItem` と `SkipCurrent` は `player` から返された履歴更新がある場合、`history.json` に保存する。

## データモデル

主要な型は `internal/domain/types.go` に定義される。

### Source enum

- `BGMSource`
  - `files`
  - `stable_audio_3`
- `TTSSource`
  - `gemini`
  - `irodori`
- `PlayableKind`
  - `bgm`
  - `talk`
  - `silence`

### AppConfig

`AppConfig` は以下の設定群を持つ。

- 基本設定:
  - `bgmRootPath`
  - `selectedGenre`
  - `rssUrls`
  - `geminiApiKey`
  - `bgmSource`
  - `ttsSource`
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
- `TTSConfig`:
  - `enabled`
  - `model`
  - `voice`
- `LocalInferenceConfig`:
  - `ortLibraryPath`
  - `maxWorkers`
  - `executionProvider`
  - `deviceId`
- `StableAudio3Config`:
  - `enabled`
  - `modelDir`
  - `outputDir`
  - `promptBase`
  - `seconds`
  - `steps`
  - `seedMode`
  - `fixedSeed`
  - `cacheLimit`
- `IrodoriConfig`:
  - `enabled`
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
- `mime`
- `title`
- `artist`
- `topicTitle`
- `durationHintMs`
- `source`

`silence` の場合は `url` を持たず、`durationHintMs` に基づいてフロントエンド側の timer で待機する。

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
- `bgmSource`: `files`
- `ttsSource`: `gemini`
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
- `tts.enabled`: `true`
- `tts.model`: `gemini-2.5-flash-preview-tts`
- `tts.voice`: `Kore`
- `localInference.maxWorkers`: `1`
- `localInference.executionProvider`: `auto`
- `localInference.deviceId`: `0`
- `stableAudio3.modelDir`: `<base>/model/sa3-sm-music`
- `stableAudio3.outputDir`: `<base>/generate_music`
- `stableAudio3.promptBase`: `instrumental background music for a radio show, seamless loop feel, no vocals`
- `stableAudio3.seconds`: `30`
- `stableAudio3.steps`: `8`
- `stableAudio3.seedMode`: `random`
- `stableAudio3.cacheLimit`: `20`
- `irodori.modelDir`: `<base>/model/irodori-v3`
- `irodori.narratorDir`: `<base>/narrator`
- `irodori.seconds`: `-1`
- `irodori.numSteps`: `40`
- `irodori.seedMode`: `random`
- `irodori.cfgText`: `3`
- `irodori.cfgCaption`: `3`
- `irodori.cfgSpeaker`: `5`
- `irodori.durationScale`: `1`

`applyConfigDefaults` は古い config で欠落した値を補完し、音量を `[0..1]` に clamp する。未知の execution provider は `cpu` に正規化される。

## 再生フロー

`internal/player.Player` が再生順序を管理する。

1. `GetNextItem` が `Player.NextItem` を呼ぶ。
2. `pendingSilence` が true の場合、まず `silence` item を返す。
3. `bgmCountSinceLastTalk >= talk.cycleBgmCount` なら Talk slot と判断する。
4. prefetched Talk があれば consume して `talk` item を返す。
5. Talk slot で prefetched Talk がなければ、同期的に `talk.Service.Generate` を試みる。
6. Talk 生成に失敗した場合は warning を log に残し、BGM へ fallback する。
7. BGM source に応じて `files` または `stable_audio_3` の item を返す。
8. BGM 再生後、Talk slot が近ければ Talk prefetch を開始する。
9. `stable_audio_3` BGM 再生後は次の Music prefetch も開始する。

`Skip` の動作:

- BGM skip は BGM count を進める。
- Talk skip は Talk slot を消費し、ready Talk を破棄する。
- Silence skip は無音を消費する。
- in-flight の Talk / Music prefetch は cancel される。

## Audio server

`internal/audio.Server` は `127.0.0.1:0` で起動し、動的 port の local HTTP server として動作する。

- path は `/audio/<token>`
- `RegisterFile(path, ttl)` は file path を token と TTL に紐づける。
- token は UUID で生成される。
- expired token は request 時と 30 秒ごとの GC loop で削除される。
- MIME type は file extension から best-effort で設定される。

## BGM 実装

### ファイル BGM

`internal/bgm` が genre と track を scan する。

- `ListGenres(root)`:
  - root 直下の directory を genre とする。
  - `.git`、`__MACOSX`、dot prefix directory は除外する。
  - 結果は sort する。
- `ListTracks(root, genre)`:
  - `root/genre` の file を走査する。
  - 対応拡張子は `.mp3`、`.wav`、`.m4a`。
- `PickRandomTrack(tracks, lastPath)`:
  - 1 曲のみならその曲を返す。
  - 複数曲なら最大 10 回、直前 path と異なる曲を選ぶ。

### Stable Audio 3

`internal/musicgen.Service` が Stable Audio 3 生成を扱う。

- `Generate(ctx, cfg, genre)`:
  - model dir と output dir を検証する。
  - execution provider を `generation.ConfigureExecutionProvider` に反映する。
  - ONNX Runtime を `generation.Init` で初期化する。
  - output dir を作成する。
  - prompt と seed を解決する。
  - `music_<unixnano>.wav` に出力する。
  - `stableaudio/pipeline` の runtime を初期化して `Synthesize` を実行する。
  - 成功後に cache trimming を行う。
- `Fallback(cfg)`:
  - output dir から fallback WAV を選ぶ。

seed 解決:

- `fixed`: `fixedSeed`
- `sequential`: current Unix time
- その他: random uint32

## Talk 実装

`internal/talk.Service` が Talk 生成を扱う。

1. `Talk.Enabled` と RSS URL の有無を確認する。
2. `rss.Picker.Pick` で未使用 article を選ぶ。
3. `llm.OpenAICompat.Complete` で Talk 原稿を作る。
4. `ttsSource` に応じて provider を選ぶ。
5. provider が WAV byte を返す。
6. `temp_audio/talk_YYYYMMDD_HHMMSS.wav` に保存する。

system prompt は、落ち着いたラジオ DJ としてニュースを 1 分で紹介する日本語口語原稿を要求する。user prompt は article title、feed title、本文を含む。本文は最大 2000 rune に制限される。

## RSS 実装

`internal/rss.Picker` の主要仕様:

- HTTP timeout: 10 秒
- 最大試行 feed 数: 5
- feed ごとの最大 item 数: 30
- 有用本文の閾値: 120 rune
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

## Gemini TTS 実装

`internal/tts.GeminiClient` が Gemini TTS を呼ぶ。

- API key は `x-goog-api-key` header に設定する。
- endpoint: `https://generativelanguage.googleapis.com/v1beta/models/<model>:generateContent`
- response modality: `AUDIO`
- voice は `generationConfig.speechConfig.voiceConfig.prebuiltVoiceConfig.voiceName` に設定する。
- default model: `gemini-2.5-flash-preview-tts`
- default voice: `Kore`
- default HTTP timeout: 90 秒
- transient request error は最大 2 回試行する。
- response audio は base64 decode し、PCM16 24 kHz mono WAV に変換する。

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

`frontend/src/App.tsx` は Wails generated API を呼び出す。

画面構成:

- Header:
  - brand
  - Talk status lamp
  - Music status lamp
  - Settings button
- Controls:
  - Genre select
  - Play / Pause button
  - Skip button
  - BGM volume range
  - Talk volume range
- Now Playing:
  - kind pill
  - title
  - subtitle
  - progress bar
  - elapsed / duration
- Settings modal:
  - BGM / TTS / Talk / LLM / ORT / Stable Audio 3 / Irodori / RSS 設定

再生:

- `GetNextItem` で次 item を取得する。
- `silence` は browser timer で duration を消費する。
- `bgm` / `talk` は returned URL を `<audio>` に設定して再生する。
- `<audio>` の `onEnded` で次 item へ進む。
- `<audio>` の `onError` でも次 item を試す。
- 再生中は 500 ms 間隔で `GetStatus` を poll する。
- 再生 progress は 250 ms 間隔で更新する。

## 開発・検証

標準コマンド:

```powershell
mise install
mise run setup
mise x -- go test ./...
mise x -- npm --prefix frontend run build
mise run build
```

ローカル生成 smoke test:

```powershell
mise x -- go run ./cmd/local_smoketest
```

CUDA 強制 smoke test:

```powershell
$env:FM_RADIO_ORT_EP='cuda'
mise x -- go run ./cmd/local_smoketest
Remove-Item Env:FM_RADIO_ORT_EP
```

`cmd/local_smoketest` は Stable Audio 3 と IrodoriTTS を短い設定で実行し、生成 WAV の sample rate、channel、frames、peak、RMS を確認する。peak または RMS が 0 以下なら失敗とする。
