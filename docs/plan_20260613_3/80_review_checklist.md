# Review Checklist

## Security

- [ ] 新しい secret、外部通信、権限追加がない。
- [ ] prompt descriptor に path や API key などの不要情報が含まれない。

## Frontend

- [ ] UI の選択肢や保存挙動は変わっていない。
- [ ] frontend build が通る。

## Backend

- [ ] `BuildPrompt` が raw genre 名だけではなく descriptor を含める。
- [ ] `GenrePromptDescription` は 4 genre すべてに対応する。
- [ ] 未対応 genre は default descriptor に fallback する。
- [ ] `source.genre` は短い genre 名を維持する。
- [ ] `source.prompt` は展開後 prompt を維持する。

## DB / Storage

- [ ] config schema に変更がない。
- [ ] `stableAudio3.genre` は短い genre 名のまま保存される。

## QA / Test

- [ ] 4 genre の descriptor 反映を test で確認する。
- [ ] 不正 genre fallback を test で確認する。
- [ ] `mise x -- go test ./...` が通る。
- [ ] `mise x -- npm --prefix frontend run build` が通る。

## DevOps / Environment

- [ ] Stable Audio 3 / ORT の環境前提を変更していない。
- [ ] `mise` 経由で検証している。

## Pre-Implementation Research

- [ ] 新規外部 dependency がないため追加 Web 調査は不要と判断している。

## Traceability

- [ ] ユーザー指摘が requirements と specification に反映されている。
- [ ] 実装後、現行 docs に as-built を反映する。
