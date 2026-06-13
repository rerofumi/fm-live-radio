# App Requirements

## Purpose

ラジオ BGM のジャンルを選択式にし、選択したジャンルが Stable Audio 3 の楽曲生成 prompt に渡される体験を提供する。

## Target Environment

- Wails + Go + React の既存デスクトップアプリ。
- 既存の Stable Audio 3 ローカル生成フロー。
- 既存 config は破壊せず、自動補完で新設定を追加する。

## User Experience

- ユーザーは BGM ジャンルを以下の 4 つから選択できる。
  - `chill lo-fi`
  - `smooth jazz`
  - `minimal electronica`
  - `ambient music`
- 選択は保存され、次回起動後も維持される。
- 選択したジャンルは Stable Audio 3 の prompt に含まれ、次回以降の BGM 生成へ反映される。
- 現在の BGM が生成された prompt または genre を確認できる。

## MVP Features

- Stable Audio 3 config に `genre` を追加する。
- 4 ジャンル以外の値が config に入った場合は既定ジャンルへ正規化する。
- Console に genre select を追加し、変更時に config を保存する。
- Settings の Stable Audio 3 生成設定にも genre select を追加する。
- `BuildPrompt` が選択 genre を Stable Audio 3 prompt へ入れる。
- `PlayableItem.Source.Genre` に生成時 genre を入れる。

## Future Candidates

- ジャンルごとの詳細 prompt preset。
- カスタムジャンル追加。
- ジャンル変更時に music prefetch をキャンセルして即時再生成する操作。
- 生成済み cache を genre ごとに分離する。

## Initial Non-Goals

- Stable Audio 3 モデルや ONNX Runtime の変更。
- 生成済み WAV の再分類。
- Talk 生成 prompt へのジャンル反映。
- 外部 Stable Audio API 連携。
