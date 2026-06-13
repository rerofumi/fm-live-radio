# App Requirements

## Purpose

ジャンル選択が Stable Audio 3 の生成結果により強く反映されるよう、各ジャンルを音楽的特徴の説明文として prompt に展開する。

## Target Environment

- 既存の Stable Audio 3 ローカル生成フロー。
- 既存の `stableAudio3.genre` config と Console / Settings UI。

## User Experience

- ユーザーはこれまで通り 4 ジャンルから選択する。
- 選択操作や保存形式は変わらない。
- 生成 prompt には選択ジャンルの音楽的特徴が含まれ、ジャンルごとの生成差が出やすくなる。
- Now Playing や `source.genre` は選択した短い genre 名を表示し、`source.prompt` には展開済み prompt を保持する。

## MVP Features

- 4 ジャンルそれぞれに英語の prompt descriptor を定義する。
- `BuildPrompt` が genre 名ではなく descriptor を prompt に含める。
- 既存の `promptBase` は descriptor の後に合成する。
- prompt test を追加または更新する。
- 現行 docs を実装後に更新する。

## Future Candidates

- UI に descriptor preview を表示する。
- descriptor をユーザー編集可能にする。
- genre ごとの seed / steps / seconds preset を追加する。
- descriptor を cheatsheet 化し、Stable Audio 3 の実生成結果と比較しながら調整する。

## Initial Non-Goals

- 新しいジャンルの追加。
- config schema の変更。
- Stable Audio 3 pipeline の変更。
- cache の genre 別分離。
