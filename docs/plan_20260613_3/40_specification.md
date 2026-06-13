# Implementation Specification

## Technical Stack

- Backend: Go
- Frontend: React 18 + TypeScript
- Desktop bridge: Wails v2
- Local generation: existing Stable Audio 3 pipeline

## File Structure

変更候補:

- `internal/musicgen/prompt.go`
  - `GenrePromptDescription(genre string) string` を追加する。
  - `BuildPrompt` は `SelectedGenre(cfg)` の genre 名ではなく `GenrePromptDescription(SelectedGenre(cfg))` を prompt に含める。
- `internal/musicgen/prompt_test.go`
  - 各 genre の descriptor が prompt に含まれることを検証する。
  - 不正 genre が default descriptor に fallback することを検証する。
- `docs/requirement.md`
  - BGM の genre は prompt descriptor に展開されることを追記する。
- `docs/specification.md`
  - prompt 構築順と descriptor 内容の概要を追記する。
- `docs/plan_20260613_3/50_review_notes.md`
  - 実装後のレビュー結果を記録する。

## Data Model

config schema は変更しない。

```go
type StableAudio3Config struct {
    Genre string `json:"genre"`
    // other existing fields...
}
```

`GenrePromptDescription` の想定 descriptor:

- `chill lo-fi`: warm lo-fi hip hop texture, dusty drums, mellow keys, soft vinyl noise, relaxed late-night mood
- `smooth jazz`: smooth jazz ensemble feel, warm electric piano, clean guitar or sax-like lead, brushed drums, relaxed sophisticated groove
- `minimal electronica`: minimal electronic composition, sparse synth patterns, precise soft pulses, restrained bass, clean modern atmosphere
- `ambient music`: ambient soundscape, slow evolving pads, airy textures, no strong beat, spacious calm immersive atmosphere

実装時は英語 descriptor として安定した文字列にする。過度な長文や UI 表示用日本語は入れない。

## UI / Screens

UI は変更しない。

- Console: 既存の Genre select を維持する。
- Settings: 既存の SA3 Genre select を維持する。
- Now Playing: `source.genre` の短い genre 名表示を維持する。

## Inputs

- 既存の `stableAudio3.genre` を入力とする。
- `SelectedGenre(cfg)` で正規化した値から descriptor を引く。

## Persistence

- descriptor は config に保存しない。
- config には既存通り短い genre 名のみ保存する。

## Error Handling

- 未対応 genre は `SelectedGenre` で `chill lo-fi` に正規化される。
- descriptor lookup が漏れた場合も default descriptor を返す。

## Import / Export

なし。

## Environment Constraints

- Stable Audio 3 model、ORT、CUDA まわりの前提は変更しない。
- ツール実行は `mise` 経由。

## Verification

1. `mise x -- go test ./...`
2. `mise x -- npm --prefix frontend run build`
3. 必要に応じて prompt test の対象文字列を確認する。
