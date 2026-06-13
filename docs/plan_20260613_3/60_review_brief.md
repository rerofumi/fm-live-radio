# Review Brief

## Review Purpose

Stable Audio 3 の genre prompt を、短い genre 名から音楽的特徴を含む descriptor へ展開する計画をレビューする。

## Review Scope

- `BuildPrompt` の prompt 構築変更。
- 4 genre の descriptor preset。
- prompt test と docs 更新。

## Decisions Needed

- D-1: `stableAudio3.genre` の保存値は短い genre 名のまま維持する。
- D-2: prompt では genre 名だけでなく descriptor を使う。
- D-3: UI は変更しない。

## Maximum Risks

- descriptor が長すぎると `promptBase` の影響が薄くなる。
- descriptor が曖昧だとジャンル差がまだ弱い。
- test が文字列固定に寄りすぎると後続調整の邪魔になる。

## Pre-Implementation Research Status

新しい外部 API や dependency はない。既存コードでは `internal/musicgen/prompt.go` の `BuildPrompt` が Stable Audio 3 pipeline に渡す prompt を構築しているため、ここを変更すれば実生成 prompt に反映される。

## Traceability Summary

| Claim | Requirement | Specification | Test / Review |
| --- | --- | --- | --- |
| ジャンル差を強める | FR-3, FR-4 | GenrePromptDescription | prompt tests |
| config/UI は維持 | FR-2, FR-7 | Data Model, UI / Screens | frontend build |
| 不正 genre fallback | FR-6 | Error Handling | prompt tests |

## Open Questions

- descriptor の最終文言は実装時に微調整可能。ただし 4 genre の音楽的差が明確であることを優先する。

## Go / No-Go

- Go: D-1 から D-3 が承認されれば実装可能。
- No-Go: descriptor をユーザー編集可能にする必要がある場合は別設計が必要。
