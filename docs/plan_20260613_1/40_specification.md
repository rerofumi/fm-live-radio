# Implementation Specification

## Technical Stack

- Backend: Go
- Frontend: React 18 + TypeScript
- Desktop: Wails v2
- Toolchain: `mise`
- VCS: `jj`

## File Structure

変更候補:

- `internal/domain/types.go`
- `internal/store/store.go`
- `internal/player/player.go`
- `internal/talk/talk.go`
- `app.go`
- `frontend/src/App.tsx`
- `frontend/src/App.css`
- `frontend/wailsjs/go/models.ts`
- `frontend/wailsjs/go/main/App.d.ts`
- `frontend/wailsjs/go/main/App.js`
- `docs/requirement.md`
- `docs/specification.md`
- `README.md`

削除候補:

- `internal/bgm/bgm.go`
- `internal/tts/gemini_tts.go`

保持候補:

- `internal/tts/provider.go`: Irodori adapter の境界として残すか、`talk` 内で不要になれば削除する。

## Data Model

`domain.AppConfig` は次の構成にする。

- `rssUrls`
- `bgmVolume`
- `talkVolume`
- `TalkConfig`
- `LLMConfig`
- `LocalInferenceConfig`
- `StableAudio3Config`
- `IrodoriConfig`

削除する field:

- `bgmRootPath`
- `selectedGenre`
- `geminiApiKey`
- `bgmSource`
- `ttsSource`
- `tts`

削除する enum:

- `BGMSource`
- `TTSSource`

`StableAudio3Config` と `IrodoriConfig` の `enabled` は、provider が固定になるため削除候補とする。既存コードで使っていなければ削除する。

## UI / Screens

### Main Console

- Genre select を削除する。
- Play / Pause、Skip、BGM volume、Talk volume、status 表示は維持する。

### Settings Modal

`modalGrid` を section 単位に分ける。

アプリ機能:

- 曲数 (BGM -> Talk)
- silence gap min / max
- BGM volume
- Talk volume
- RSS URLs
- LLM base URL
- LLM API key
- LLM model

生成設定:

- `details` / `summary` を使う。
- `details` は `open` を付けず、初期状態で閉じる。
- ORT DLL Path
- Local Inference Provider
- Local Inference Device ID
- Stable Audio 3 Model Dir
- Stable Audio 3 Output Dir
- Stable Audio 3 Prompt Base
- Stable Audio 3 Seconds
- Stable Audio 3 Steps
- Stable Audio 3 seed mode / fixed seed / cache limit
- Irodori Model Dir
- Irodori Narrator Dir
- Irodori Ref WAV
- Irodori Steps
- Irodori seed mode / fixed seed
- Irodori CFG values
- Irodori Duration Scale

## Inputs

- `NextItemRequest` と `SkipRequest` の `selectedGenre` は削除する。
- Frontend から `GetNextItem({})` / `SkipCurrent({ currentKind })` 相当の request を送る。
- `PrefetchTalk` は genre を渡さずに `PrefetchMusic` する。

## Persistence

- `LoadConfig` は旧 JSON field を無視して読み込む。
- `SaveConfig` は新 struct を marshal するため旧 field を保存しない。
- 明示的な migration file は作らない。

## Error Handling

- Stable Audio 3 生成失敗時の fallback は現行 `musicgen.Fallback` を維持する。
- IrodoriTTS 生成失敗時は現行 Talk 生成失敗として扱い、player は BGM fallback へ進む。
- 旧 provider 未設定エラーは削除する。

## Import / Export

- Go API 変更後に Wails binding を更新する。
- Wails CLI が未導入なら `mise run setup` を実行してから生成する。

## Environment Constraints

- `go`, `npm`, `wails` は `mise` 経由で実行する。
- ONNX Runtime とモデルファイルはリポジトリ外または Git 管理外に置く前提を維持する。

## Verification

実装後に次を実行する。

```powershell
mise x -- go test ./...
mise x -- npm --prefix frontend run build
```

Wails binding 更新が必要な場合:

```powershell
mise x -- wails generate module
```

必要に応じて:

```powershell
mise run build
```
