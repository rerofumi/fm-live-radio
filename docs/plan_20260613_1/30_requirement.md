# Requirements

## Scope

旧 provider と関連設定を削除し、設定 UI をローカル生成専用に再編する。

## Functional Requirements

- FR-1: AppConfig は `bgmRootPath`、`selectedGenre`、`geminiApiKey`、`bgmSource`、`ttsSource`、Gemini 用 `tts` config を持たない。
- FR-2: 既存 `config.json` に旧 field が含まれていても load は失敗しない。
- FR-3: 新規保存される `config.json` には旧 field が出力されない。
- FR-4: BGM は常に Stable Audio 3 生成を使う。
- FR-5: Stable Audio 3 生成に失敗した場合、生成済み output dir の WAV fallback は維持する。
- FR-6: Talk 音声は常に IrodoriTTS v3 で合成する。
- FR-7: Settings は「アプリ機能」と「生成設定」に分かれている。
- FR-8: 「生成設定」は初期表示で閉じている。
- FR-9: Settings から RSS、LLM、Talk 周期、音量、無音 gap、ORT、Stable Audio 3、IrodoriTTS の設定を保存できる。
- FR-10: 再生画面からファイル BGM genre 選択 UI は削除される。
- FR-11: Backend API からファイル BGM scan 用の `ScanGenres` を削除する。
- FR-12: Gemini TTS 実装は参照されなくなり、不要ファイルを削除できる。

## Non-Functional Requirements

- NFR-1: `mise x -- go test ./...` が通る。
- NFR-2: `mise x -- npm --prefix frontend run build` が通る。
- NFR-3: 旧 provider 削除後も再生ループ、prefetch、local generation status は維持する。
- NFR-4: 設定 UI は既存の明るいミニマルなデザインに合わせる。

## Constraints

- `mise.toml` が存在するため、検証コマンドは `mise` 経由で実行する。
- `.jj` が存在するため、VCS 状態確認は `jj` を優先する。
- コード修正前にこの plan のレビュー承認を得る。

## Compatibility

- 既存 config の旧 field は JSON unmarshal 時に無視される。
- 旧 config で `bgmSource=files` や `ttsSource=gemini` が保存されていても、新しい runtime はローカル生成固定として動作する。
- 旧 config の `selectedGenre` は削除され、Stable Audio 3 prompt には genre を渡さない。

## Out of Scope

- 旧 provider の runtime fallback。
- Gemini API の設定保持。
- BGM root / genre の選択、scan、再生。
- 生成モデルの自動ダウンロード。

## Acceptance Criteria

- AC-1: Settings に `BGM Source`、`TTS Source`、`BGM Root Path`、`Selected Genre`、`Gemini API Key`、`TTS Model`、`TTS Voice` が表示されない。
- AC-2: Settings に「アプリ機能」と「生成設定」があり、「生成設定」は閉じた状態で開く。
- AC-3: 保存した config JSON に `bgmRootPath`、`selectedGenre`、`geminiApiKey`、`bgmSource`、`ttsSource`、`tts` が含まれない。
- AC-4: `internal/player` はファイル BGM 分岐を持たない。
- AC-5: `internal/talk` は Gemini provider 分岐を持たない。
- AC-6: `internal/bgm` と `internal/tts/gemini_tts.go` は参照されないか削除される。
- AC-7: `mise x -- go test ./...` が成功する。
- AC-8: `mise x -- npm --prefix frontend run build` が成功する。
