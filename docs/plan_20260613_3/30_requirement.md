# Requirements

## Scope

Stable Audio 3 の BGM prompt 構築において、選択 genre を説明文 preset に展開する。UI、config、保存値は既存仕様を維持する。

## Functional Requirements

- FR-1: 許可ジャンルは既存通り `chill lo-fi`, `smooth jazz`, `minimal electronica`, `ambient music` の 4 値である。
- FR-2: `stableAudio3.genre` の保存値は短い genre 名のまま維持する。
- FR-3: `BuildPrompt` は genre 名だけでなく、その genre の音楽的特徴を説明する descriptor を prompt に含める。
- FR-4: descriptor は楽器・音色・リズム・雰囲気・ミックス傾向のうち複数を含む。
- FR-5: `source.genre` は短い genre 名、`source.prompt` は descriptor 展開後の実 prompt を保持する。
- FR-6: 不正 genre は既存通り `chill lo-fi` に正規化され、その descriptor が使われる。
- FR-7: 既存 UI の選択肢、保存操作、非中断挙動は変えない。

## Non-Functional Requirements

- NFR-1: prompt descriptor は deterministic でテスト可能である。
- NFR-2: descriptor は長くしすぎず、既存 `promptBase` と共存できる。
- NFR-3: 既存 config 互換を維持する。
- NFR-4: BGM / Talk 再生フローに影響を与えない。

## Constraints

- `mise.toml` があるため検証は `mise` 経由で行う。
- `.jj` があるためリポジトリ状態確認は `jj` を優先する。
- コード変更前にこの plan の承認を得る。

## Compatibility

- `stableAudio3.genre` の JSON field は変えない。
- UI の 4 択 select は変えない。
- 既存の `promptBase` は引き続き prompt に含める。

## Out of Scope

- prompt descriptor のユーザー編集 UI。
- Stable Audio 3 の生成パラメータ自動変更。
- 生成済み cache の invalidation。

## Acceptance Criteria

- `BuildPrompt` で `chill lo-fi` を選ぶと lo-fi の特徴説明が含まれる。
- `BuildPrompt` で `smooth jazz` を選ぶと jazz の特徴説明が含まれる。
- `BuildPrompt` で `minimal electronica` を選ぶと minimal/electronic の特徴説明が含まれる。
- `BuildPrompt` で `ambient music` を選ぶと ambient の特徴説明が含まれる。
- prompt に既存 `promptBase`, `instrumental`, `background music`, `no vocals` が残る。
- `mise x -- go test ./...` が成功する。
- `mise x -- npm --prefix frontend run build` が成功する。
