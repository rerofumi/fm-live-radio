# Review Checklist

## Security

- [ ] 新しい secret、外部通信、権限追加がない。
- [ ] Stable Audio 3 prompt に API key や path などの不要情報を含めない。

## Frontend

- [ ] Console で 4 ジャンルを選択できる。
- [ ] Settings の SA3 Genre と Console の選択値が同じ config を編集する。
- [ ] モバイル幅で select、mixer、transport が重ならない。
- [ ] 現在の BGM 表示に genre が含まれる。

## Backend

- [ ] `StableAudio3Config` に `genre` が追加されている。
- [ ] 旧 config で genre が欠落していても default 補完される。
- [ ] 未対応 genre が default に正規化される。
- [ ] `BuildPrompt` が genre を Stable Audio 3 prompt に含める。

## DB / Storage

- [ ] `config.json` へ `stableAudio3.genre` が保存される。
- [ ] 既存 config の読み込み互換性が保たれている。

## QA / Test

- [ ] `mise x -- go test ./...` が通る。
- [ ] `mise x -- npm --prefix frontend run build` が通る。
- [ ] Wails generated bindings が Go 型と一致している。

## DevOps / Environment

- [ ] `mise.toml` の task 方針に従って検証している。
- [ ] Stable Audio 3 / ORT の環境前提を変更していない。

## Pre-Implementation Research

- [ ] 現行 prompt flow が `BuildPrompt` から Stable Audio 3 pipeline に渡ることを確認済み。
- [ ] 外部 Web 依存の新規仕様がない。

## Traceability

- [ ] `10_claim.md` の目的が `30_requirement.md` に落ちている。
- [ ] `30_requirement.md` の各 FR が `40_specification.md` に対応している。
- [ ] 実装後に `docs/requirement.md` と `docs/specification.md` へ as-built を反映する。
