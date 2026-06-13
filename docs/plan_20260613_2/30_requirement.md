# Requirements

## Scope

Stable Audio 3 による BGM 生成に対して、ユーザー選択式の genre を追加する。対象は config、prompt 構築、UI、表示、検証である。

## Functional Requirements

- FR-1: アプリは `chill lo-fi`, `smooth jazz`, `minimal electronica`, `ambient music` の 4 ジャンルを提供する。
- FR-2: 選択ジャンルは `config.json` の Stable Audio 3 設定として保存される。
- FR-3: 旧 config に genre がない場合、既定値は `chill lo-fi` になる。
- FR-4: config に未対応 genre がある場合、保存・読み込み時に `chill lo-fi` へ正規化される。
- FR-5: Stable Audio 3 に渡す prompt は選択 genre を含む。
- FR-6: `promptBase` は廃止せず、選択 genre と合成して引き続き使える。
- FR-7: ユーザーはメイン画面の操作領域から genre を変更できる。
- FR-8: ユーザーは Settings の生成設定からも同じ genre を確認・変更できる。
- FR-9: 生成された BGM item の source には genre と prompt が含まれる。
- FR-10: 再生中に genre を変更しても、現在再生中または既に準備済みの BGM は中断しない。

## Non-Functional Requirements

- NFR-1: 既存 config との後方互換性を維持する。
- NFR-2: UI は既存の明るいミニマルなデザインに合わせ、レイアウト崩れを起こさない。
- NFR-3: prompt 構築は deterministic でテスト可能にする。
- NFR-4: 既存の BGM / Talk 再生フローを変えない。

## Constraints

- Wails binding の生成ファイルは Go 型変更に合わせて更新する必要がある。
- `mise.toml` があるため検証コマンドは `mise` 経由で実行する。
- `.jj` が存在するため、リポジトリ状態確認は `jj` を優先する。

## Compatibility

- 既存の `stableAudio3.promptBase` はそのまま有効。
- 既存 config の欠落 field は `applyConfigDefaults` で補完する。
- 旧 UI で保存済みの設定でも新 UI で読み込める。

## Out of Scope

- 生成済み WAV cache の genre 別管理。
- Stable Audio 3 の inference pipeline 改修。
- ジャンルごとの seed 戦略。
- カスタム prompt preset editor。

## Acceptance Criteria

- `BuildPrompt` の結果に選択 genre が含まれる。
- `LoadConfig` が genre 欠落 config を `chill lo-fi` に補完する。
- UI で 4 ジャンルを選択でき、保存後に config に反映される。
- BGM 生成結果の `source.genre` と `source.prompt` が UI データに含まれる。
- `mise x -- go test ./...` が実行可能である。
- `mise x -- npm --prefix frontend run build` が実行可能である。
