# Claim

## Purpose

ユーザーがラジオ BGM の音楽ジャンルを UI から選択し、選択したジャンルを Stable Audio 3 の生成 prompt に反映して楽曲生成できるようにする。

## Background

現行実装では Stable Audio 3 の `promptBase` を Settings で編集できるが、再生体験中にジャンルを選ぶ専用 UI はない。直近の計画ではファイル BGM 由来の genre UI を削除しているため、Stable Audio 3 用の明示的なジャンル選択として再設計する必要がある。

## Problem

- ユーザーは BGM の方向性を `"chill lo-fi"`, `"smooth jazz"`, `"minimal electronica"`, `"ambient music"` から簡単に切り替えたい。
- 現行の `promptBase` だけでは、ジャンル切り替えが設定文字列編集になり、ラジオ操作として扱いにくい。
- Stable Audio 3 へ渡す prompt に選択ジャンルが確実に含まれる仕様が必要である。

## Target Users / Environment

- Windows x64 の Wails デスクトップアプリ利用者。
- Stable Audio 3 のローカルモデルと ONNX Runtime を設定済み、または設定予定のユーザー。

## Initial Scope

- `StableAudio3Config` に選択ジャンルを保存する。
- Settings またはメイン操作面にジャンル選択 UI を追加する。
- `BuildPrompt` で選択ジャンルを prompt に含める。
- 生成済み `PlayableItem.Source` に genre と prompt を反映し、UI で現在の BGM の方向性が追えるようにする。

## Initial Technical Hypotheses

- 既存の `StableAudio3Config.PromptBase` は残し、`Genre` を追加して `genre, promptBase, instrumental, background music, no vocals` の順で prompt を組み立てる。
- 既存 config 互換のため、旧 config に genre がない場合は `"chill lo-fi"` を既定値にする。
- Wails の TS binding は Go 型更新後に `wails generate module` または build 相当で更新できる。

## Uncertainties

- ジャンル UI をメイン画面に置くか Settings に置くか。MVP では再生中に切り替えやすいよう Console に置き、Settings の生成設定にも同値を表示する案を採用する。
- 再生中にジャンル変更した場合、既に prefetched / cached された BGM を破棄するか。MVP では次回生成から反映し、既存 ready item の破棄はしない。

## Next Documents

1. `20_app_requirement.md`
2. `30_requirement.md`
3. `40_specification.md`
4. `50_review_notes.md`
5. `60_review_brief.md`
6. `70_review_board.html`
7. `80_review_checklist.md`
