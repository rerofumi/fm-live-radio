# Review Notes

## Review Findings

- 実装完了。ユーザー指摘に基づき、genre 名だけではなく genre descriptor を Stable Audio 3 prompt に含めるように更新した。
- config と UI は短い genre 名のまま維持し、prompt 構築時だけ descriptor に展開する。

## Fixed Items

- `internal/musicgen/prompt.go` に `GenrePromptDescription` を追加した。
- `BuildPrompt` は `GenrePromptDescription(SelectedGenre(cfg))` を prompt 先頭に置く。
- 4 genre それぞれについて、楽器・音色・リズム・雰囲気を含む英語 descriptor を定義した。
- `internal/musicgen/prompt_test.go` を更新し、descriptor 展開と fallback を検証する。
- `docs/requirement.md` と `docs/specification.md` に as-built の descriptor 展開仕様を反映した。

## Deferred Items

- descriptor preview UI。
- descriptor のユーザー編集。
- 実生成音源を比較した descriptor の再調整。

## Rejected Options

- config に descriptor を保存する案は採用しない。現段階では preset としてコード管理した方が後方互換性と UI の単純さを保てるため。
- genre 名を prompt から完全に消して descriptor だけにする案は採用しない。descriptor 内に genre 文脈を自然に含める。

## Current Non-Goals

- 新ジャンル追加。
- Stable Audio 3 pipeline や generation parameter の変更。
- cache invalidation。

## Future Plans

- 生成結果を聴感レビューして descriptor を調整する。
- genre preset の versioning。

## Next Plan Candidates

- Stable Audio 3 prompt preview UI。
- genre preset editor。

## Documentation Feedback

- 反映済み。
