# Review Checklist

## Security

- [ ] 新規 secret、外部通信、認証情報の扱いが増えていない。
- [ ] 画像生成アセットに秘密情報やユーザー固有パスが埋め込まれない。

## Frontend

- [ ] Play/Pause、Skip、BGM/Talk volume、Genre、Settings が操作可能。
- [ ] ツマミ、パイロット LED、ジャンルチューニング、波形窓、右上 Settings が確認できる。
- [ ] デスクトップ幅と狭幅でテキストやボタンが重ならない。
- [ ] 起動時 window size でラジオ筐体全体と主要操作が見切れない。
- [ ] window を広げても筐体が過度に引き伸ばされない。
- [ ] window を狭めると 2 カラムから縦積みに変化し、横スクロール前提にならない。
- [ ] 画像アセットなしでも主要情報が読める。
- [ ] `prefers-reduced-motion` で過度な pulse が止まる。

## Backend

- [ ] Wails API と Go 型の変更が不要である。
- [ ] `main.go` の変更は Wails `options.App` の window size/min size に限定される。
- [ ] 再生順序、prefetch、loudness envelope 取得に影響していない。

## DB / Storage

- [ ] 設定保存形式に変更がない。
- [ ] Genre 更新は既存 `UpdateStableAudio3Genre` を使う。

## QA / Test

- [ ] `mise x -- npm --prefix frontend run build` が成功する。
- [ ] 可能なら `mise run dev` または Vite dev server で表示確認する。
- [ ] Play/Pause/Skip/Volume/Genre/Settings の手動 smoke test を行う。

## DevOps / Environment

- [ ] コマンドは `mise` 経由で実行される。
- [ ] VCS 操作は `.jj` があるため `jj` を使う。

## Pre-Implementation Research

- [ ] 現行 UI と Visualizer の構造を確認済み。
- [ ] 外部 API 調査が不要な理由が明記されている。

## Traceability

- [ ] Claim、Requirement、Specification、Review の対応に抜けがない。
- [ ] 実装後に `docs/requirement.md` と `docs/specification.md` を更新する。
