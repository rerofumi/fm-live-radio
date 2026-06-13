# Claim

## Purpose

Stable Audio 3 に渡す楽曲生成 prompt で、選択ジャンル名だけでなく、そのジャンルの音楽的特徴を説明する文章を追加し、ジャンル間の生成差を強くする。

## Background

`docs/plan_20260613_2` では Stable Audio 3 の genre 選択 UI と config 保存を追加した。しかし現行実装の `BuildPrompt` は `chill lo-fi` などのジャンル名を prompt に入れるだけであり、Stable Audio 3 への指示としては弱い。

## Problem

- ジャンル名だけでは生成モデルに十分な音楽的制約が伝わらない。
- `chill lo-fi`, `smooth jazz`, `minimal electronica`, `ambient music` の差が出にくい。
- ユーザーはジャンルごとに楽器、音色、テンポ感、リズム、雰囲気の違いが prompt に含まれることを期待している。

## Target Users / Environment

- Stable Audio 3 のローカル生成 BGM を使う Windows desktop app 利用者。
- 既存の Wails + Go + React 構成。

## Initial Scope

- 既存の `stableAudio3.genre` の保存値と UI は維持する。
- prompt 構築時に genre を説明文 preset に展開する。
- `source.genre` は選択値のまま維持する。
- `source.prompt` には展開後の実 prompt を入れる。
- prompt unit test を更新し、ジャンル名だけでなく説明文が含まれることを確認する。

## Initial Technical Hypotheses

- `internal/musicgen/prompt.go` に `GenrePromptDescription(genre string) string` のような helper を追加する。
- `BuildPrompt` は `GenrePromptDescription(SelectedGenre(cfg))` を先頭に置き、その後に `promptBase`, `instrumental`, `background music`, `no vocals` を続ける。
- config schema の変更は不要。

## Uncertainties

- どの程度長い説明文が Stable Audio 3 に対して最適かはモデル依存である。MVP では 1 ジャンルあたり 1 文から 2 文の英語 descriptor とし、過度に長い prompt は避ける。
- BPM のような数値を入れるかは検討対象。MVP では雰囲気と編成を優先し、固定 BPM は必要最小限にする。

## Next Documents

1. `20_app_requirement.md`
2. `30_requirement.md`
3. `40_specification.md`
4. `50_review_notes.md`
5. `60_review_brief.md`
6. `70_review_board.html`
7. `80_review_checklist.md`
