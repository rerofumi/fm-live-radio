# Review Brief

## Review Purpose

コード修正前に、ファイル BGM と Gemini TTS を削除し、Settings / config をローカル生成専用に整理する方針を確認する。

## Review Scope

- Backend config model
- Player BGM selection
- Talk TTS provider selection
- Wails API / generated binding
- Settings modal layout
- Current docs / README update

## Decisions Needed

1. 旧 provider は UI だけでなく backend からも削除する。
2. 既存 config の旧 field は読み捨て、新規保存時に消える方式にする。
3. Settings は「アプリ機能」と「生成設定」に分け、「生成設定」は閉じた details とする。
4. Genre concept は今回削除し、Stable Audio 3 prompt 分類は将来 plan に回す。

## Maximum Risks

- Wails binding 更新がローカル環境の Wails CLI 状態に依存する。
- `selectedGenre` 削除により、既存の genre 別 prompt 風運用ができなくなる。
- ローカル生成未設定環境では旧 fallback がなくなるため、再生開始時に生成設定エラーが出やすくなる。

## Pre-Implementation Research Status

- 外部 API の新規採用はない。
- ローカルコード確認で、旧 provider は `domain`、`store`、`player`、`talk`、`app.go`、`frontend/src/App.tsx`、Wails generated models にまたがることを確認済み。
- 検証コマンドは既存 docs と `mise.toml` から確認済み。

## Traceability Summary

| Claim | Requirement | Specification | Test / Review |
| --- | --- | --- | --- |
| 旧 provider 削除 | FR-1, FR-4, FR-6, FR-11, FR-12 | Data Model, File Structure | `go test`, code review |
| config 簡素化 | FR-2, FR-3 | Persistence | saved JSON review |
| Settings 再編 | FR-7, FR-8, FR-9 | UI / Screens | frontend build, visual review |
| Genre 削除 | FR-10 | Main Console, Inputs | frontend build |

## Open Questions

- `StableAudio3Config.enabled` と `IrodoriConfig.enabled` は今回削除してよいか。仕様上は provider 固定なので削除候補。

## Go / No-Go

Go 条件:

- この plan の方針にユーザー承認がある。
- `enabled` field の削除可否が決まっている。未回答の場合は「固定 provider なので削除」で進める。
