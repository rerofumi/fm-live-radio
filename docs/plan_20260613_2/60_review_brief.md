# Review Brief

## Review Purpose

Stable Audio 3 の BGM 生成に対し、4 択のラジオ音楽ジャンル選択を追加してよいかをレビューする。

## Review Scope

- Config schema への `stableAudio3.genre` 追加。
- UI の genre select 追加。
- Stable Audio 3 prompt への genre 反映。
- 既存 config 互換と検証方法。

## Decisions Needed

- D-1: 既定ジャンルを `chill lo-fi` にする。
- D-2: genre 変更は次回生成から反映し、現在再生中または prefetched BGM は破棄しない。
- D-3: `promptBase` は残し、genre と合成する。

## Maximum Risks

- Wails binding 更新漏れで frontend build が失敗する。
- 既存 config 互換処理が不足すると genre が空の prompt になる。
- Console に select を追加して狭い画面でレイアウトが崩れる。

## Pre-Implementation Research Status

外部調査は不要。現行コードで Stable Audio 3 prompt は `internal/musicgen/prompt.go` の `BuildPrompt` から pipeline options に渡されていることを確認済み。

## Traceability Summary

| Claim | Requirement | Specification | Test / Review |
| --- | --- | --- | --- |
| 4 ジャンルを選べる | FR-1, FR-7, FR-8 | UI / Screens | frontend build, manual UI |
| 選択 genre を prompt に渡す | FR-5, FR-6 | Data Model, Inputs | Go test for BuildPrompt |
| 既存 config を壊さない | FR-2, FR-3, FR-4 | Persistence, Error Handling | Go test / load config review |
| 生成 item で追跡できる | FR-9 | File Structure, UI / Screens | manual item subtitle/source check |

## Open Questions

- Console の配置は transport row 右側でよいか。仕様ではこの案を採用している。

## Go / No-Go

- Go: D-1 から D-3 が承認されれば実装可能。
- No-Go: ジャンル変更を即時に現在の生成キューへ反映する必要がある場合は、prefetch cancellation の追加設計が必要。
