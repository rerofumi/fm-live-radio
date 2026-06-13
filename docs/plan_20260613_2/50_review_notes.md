# Review Notes

## Review Findings

- 旧 plan のレビューを反映して実装完了。`docs/requirement.md` と `docs/specification.md` は実装済み挙動に更新済み。
- 旧 plan のレビュー時指摘事項はすべて解消:
  - `BuildPrompt` は `cfg.StableAudio3.Genre` を直接使わず、`store.NormalizeStableAudio3Genre` の出力（`musicgen.SelectedGenre`）を prompt 先頭に置く。`applyConfigDefaults` パスを通らない不正ジャンル値が prompt に漏れる経路がない。
  - `Player.UpdateStableAudio3Genre` を追加し、`App.UpdateStableAudio3Genre` Wails バインディング経由で prefetch / cycle reset を起こさずジャンルだけ更新する。Console の `Genre` select はこのバインディングを呼ぶ。
  - Settings の `SA3 Genre` select は `AppConfig.stableAudio3.genre` を編集し、Settings の Save ボタンで `App.SaveConfig` 経由で config.json に保存される。
  - `PlayableItem.Source` に `genre` と `prompt` を必ず含める（`Player.pickBGM`）。
  - Wails 生成バインディング `frontend/wailsjs/go/models.ts` の `StableAudio3Config` に `genre: string` フィールドとコンストラクタ代入を追加。`App.d.ts` / `App.js` に `UpdateStableAudio3Genre(arg1:string)` を追加。
  - Now Playing の BGM subtitle は `BGM · stable_audio_3 · <genre>` の形式で `source.provider` と `source.genre` を表示する。provider/genre が欠ける場合は存在する項目のみ ` · ` 区切り。

## Fixed Items

- 旧 `internal/bgm`、`internal/tts` 由来の `ScanGenres`、`BGMSource`、`TTSSource`、`GeminiTTS`、`selectedGenre` request などの旧 API は本 plan のスコープ外として持ち込まない。
- BGM アイテムの `PlayableSource` に `genre` を必ず入れるように `Player.pickBGM` を更新。
- Wails 生成バインディング `models.ts` を Go 型（`internal/domain/types.go` の `StableAudio3Config.Genre`）と一致するように手動更新。

## Deferred Items

- ジャンル変更時に既存 music prefetch を破棄するかは deferred（FR-10）。MVP は次回生成から反映する。Console からの `App.UpdateStableAudio3Genre` 呼び出しでは prefetch を破棄しない。
- cache を genre 別に分けるかは deferred。
- ジャンルごとの prompt preset 拡張は deferred。

## Rejected Options

- `promptBase` をジャンルごとに丸ごと置換する案は採用しない。ユーザーが既存の共通 prompt を保てなくなるため。
- 自由入力 genre は採用しない。今回の要求は 4 ジャンル固定であり、UI と prompt の安定性を優先するため。
- `BuildPrompt` 内で `cfg.StableAudio3.Genre` を直接参照する案は採用しない。default / 正規化を経由した値のみを prompt に使う（`musicgen.SelectedGenre`）。

## Current Non-Goals

- Stable Audio 3 pipeline の音質調整。
- 外部 API 化。
- Talk 側の演出変更。
- BGM cache metadata への genre/prompt 記録。

## Future Plans

- genre preset ごとの prompt descriptor 追加。
- cache metadata に genre / prompt / seed を保存して履歴表示する。
- Console のレイアウトに合わせた genre 追加 UI（拡張時）。

## Next Plan Candidates

- BGM 生成キューと genre 変更の即時反映。
- Stable Audio 3 生成結果の管理画面。
- genre 単位の prefetch キャンセル / 再投入。

## Documentation Feedback

- 実装完了後、`docs/requirement.md` の BGM 機能要件にジャンル仕様（4 値 / 既定 / 正規化）を追記済み。
- 実装完了後、`docs/specification.md` の `StableAudio3Config` に `genre` を追加し、default・prompt 構築順・Player 側の source 反映・`UpdateStableAudio3Genre` API・UI (Console / Settings / Now Playing) を追記済み。

## Implementation Notes (as-built)

- バックエンド:
  - `internal/domain/types.go`: `StableAudio3Config.Genre string` を追加。`PlayableSource` には既に `Genre` と `Prompt` フィールドがあった。
  - `internal/store/store.go`: `StableAudio3AllowedGenres`（4 値）と `StableAudio3DefaultGenre` (`chill lo-fi`) を追加。`NormalizeStableAudio3Genre` でトリム + 大文字小文字無視 + 完全一致に正規化。`applyConfigDefaults` 内で `cfg.StableAudio3.Genre = NormalizeStableAudio3Genre(cfg.StableAudio3.Genre)` を実行。`DefaultConfig` にも `Genre: StableAudio3DefaultGenre` を設定。
  - `internal/musicgen/prompt.go`: `BuildPrompt` は `SelectedGenre(cfg)` を prompt 先頭に置く。`SelectedGenre` は `store.NormalizeStableAudio3Genre` の薄いラッパー。
  - `internal/musicgen/service.go`: `Result` に `Genre` を追加。`Generate` および `Fallback` の戻り値に正規化済み `Genre` を設定。
  - `internal/player/player.go`: `Player.UpdateStableAudio3Genre(genre string)` を追加。prefetch / cycle reset を行わず cfg のみ更新。`pickBGM` の `PlayableSource` に `Genre: res.Genre` を設定。
  - `app.go`: `App.UpdateStableAudio3Genre(genre string)` Wails バインディングを追加。`store.SaveConfig` で永続化し、`Player.UpdateStableAudio3Genre` を呼ぶ。`App.SaveConfig` は従来通り全 config を保存（prefetch を破棄する UpdateConfig を呼ぶ）。
- テスト:
  - `internal/musicgen/prompt_test.go`: `BuildPrompt` がジャンル + promptBase を含むか、promptBase 維持、空/不正ジャンルの正規化を検証。
  - `internal/store/store_test.go`: `NormalizeStableAudio3Genre` の空・trim・大小無視・不正値の挙動と、許可ジャンルが 4 件であることを検証。
- フロントエンド:
  - `frontend/src/App.tsx`:
    - import に `UpdateStableAudio3Genre` を追加。
    - `SA3_GENRES` (4 値)、`SA3_DEFAULT_GENRE`、`normalizeSa3Genre` ヘルパーを追加。
    - `AppConfig.stableAudio3` に `genre: string` を追加、`PlayableItem.source` に `genre?: string` を追加。
    - `changeGenre` 関数を追加し `App.UpdateStableAudio3Genre` を呼ぶ。
    - Console の mixer 下に `Genre` select を追加（`UpdateStableAudio3Genre` 呼び出し）。
    - Settings の `SA3 Genre` select を `SA3 Prompt Base` の前に追加（`AppConfig.stableAudio3.genre` を編集）。
    - `nowSub` を `BGM · <provider> · <genre>` 形式に更新。
  - `frontend/src/App.css`: `.genreSelect` のスタイルを追加（既存 `.genre` を流用しつつ、select を見やすく整形）。
  - `frontend/wailsjs/go/main/App.{js,d.ts}`: `UpdateStableAudio3Genre(arg1:string)` を手動で追加。
  - `frontend/wailsjs/go/models.ts`: `StableAudio3Config` に `genre: string` と `this.genre = source["genre"]` を追加。
- ドキュメント:
  - `docs/requirement.md` の BGM 機能要件にジャンル仕様（4 値 / 既定 / 正規化）を追記。
  - `docs/specification.md` の `StableAudio3Config` フィールド、default、applyConfigDefaults、BGM 実装（ジャンル仕様 / prompt 構築順 / プレイヤー source 反映）、ジャンル変更 API、UI/Console、UI/Settings、Now Playing subtitle を追記。
- 検証:
  - `mise x -- go test ./...` → `ok fm-live-radio/internal/musicgen`, `ok fm-live-radio/internal/store`。
  - `mise x -- npm --prefix frontend run build` → `vite build` 成功 (TypeScript エラーなし)。
