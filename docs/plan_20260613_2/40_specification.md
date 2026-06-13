# Implementation Specification

## Technical Stack

- Backend: Go
- Frontend: React 18 + TypeScript
- Desktop bridge: Wails v2
- Local generation: existing Stable Audio 3 pipeline

## File Structure

変更候補:

- `internal/domain/types.go`
  - `StableAudio3Config.Genre string json:"genre"` を追加する。
- `internal/store/store.go`
  - Stable Audio 3 genre の既定値と正規化を追加する。
- `internal/musicgen/prompt.go`
  - genre を prompt parts の先頭へ入れる。
  - 許可 genre の helper を追加する場合は同 file または store 側に閉じる。
- `internal/musicgen/service.go`
  - `Result` に `Genre` を追加する、または player 側で config から source に genre を入れる。
- `internal/player/player.go`
  - BGM item source に `Genre` を入れる。
- `frontend/src/App.tsx`
  - `AppConfig.stableAudio3.genre` を TS type に追加する。
  - genre option 定数を追加する。
  - Console と Settings に select を追加する。
  - 現在 BGM subtitle に genre を含める。
- `frontend/src/App.css`
  - 既存 `.genre` style を活用または調整し、モバイル崩れを防ぐ。
- `frontend/wailsjs/go/models.ts`
  - Wails 生成結果を更新する。
- `docs/requirement.md`, `docs/specification.md`
  - 実装後に as-built のみ反映する。

## Data Model

`StableAudio3Config`:

```go
type StableAudio3Config struct {
    ModelDir   string  `json:"modelDir"`
    OutputDir  string  `json:"outputDir"`
    PromptBase string  `json:"promptBase"`
    Genre      string  `json:"genre"`
    Seconds    float64 `json:"seconds"`
    Steps      int     `json:"steps"`
    SeedMode   string  `json:"seedMode"`
    FixedSeed  uint32  `json:"fixedSeed"`
    CacheLimit int     `json:"cacheLimit"`
}
```

Allowed genres:

- `chill lo-fi`
- `smooth jazz`
- `minimal electronica`
- `ambient music`

Default genre:

- `chill lo-fi`

## UI / Screens

Console:

- Transport row の右側、または mixer の上に `Genre` select を置く。
- 選択肢は 4 つのみ。
- 変更時は `persistConfig({...cfg, stableAudio3: {...cfg.stableAudio3, genre}})` を呼び、次回生成から反映する。

Settings:

- 生成設定 details 内の `SA3 Prompt Base` 付近に `SA3 Genre` select を追加する。
- Console と同じ config field を編集する。

Now Playing:

- BGM subtitle は `BGM · stable_audio_3 · chill lo-fi` のように source genre を含める。

## Inputs

- select からのみ genre を入力する。
- config file 由来の未知 genre は backend で既定値へ正規化する。

## Persistence

- `config.json` の `stableAudio3.genre` に保存する。
- 旧 config は読み込み時に default 補完する。

## Error Handling

- 未対応 genre は error にせず default へ正規化する。
- promptBase が空でも default promptBase を補完する既存挙動を維持する。

## Import / Export

なし。

## Environment Constraints

- Stable Audio 3 モデル、ORT、CUDA まわりの前提は現行仕様を維持する。
- ツール実行は `mise` 経由。

## Verification

1. `mise x -- go test ./...`
2. `mise x -- npm --prefix frontend run build`
3. 必要に応じて `mise run build`
4. 手動確認:
   - Settings を開き、SA3 Genre が 4 ジャンルから選べる。
   - Console で Genre を変更すると config が保存される。
   - 生成後 item の subtitle または source prompt に genre が反映される。
