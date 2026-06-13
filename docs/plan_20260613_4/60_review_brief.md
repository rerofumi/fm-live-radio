# Review Brief

## Review Purpose

Visualizer を実音声の loudness に反応させる実装方式を選ぶための調査結果をレビューする。

## Review Scope

- Web Audio API で RMS を取る方式。
- Go backend で WAV envelope を計算する方式。
- 既存 Wails / React / local audio server 構成への影響。

## Decisions Needed

- D-1: MVP は Option A: backend WAV envelope で進める。
- D-2: WAV 対応範囲は既存生成経路に合わせ、16-bit PCM に絞る。
- D-3: loudness endpoint には常時 CORS ヘッダを付けるか。
- D-4: 視覚ズレ対策として、envelope 全体を事前取得し `<audio>.currentTime` で参照する方式を採る。

## Maximum Risks

- WAV decoder 対応漏れで loudness が常に fallback になる。
- token / skip 切替時に古い loudness 値が残る。
- Web Audio 方式を採った場合、CORS silence または AudioContext lifecycle で再生経路を壊す。

## Pre-Implementation Research Status

公式ソース確認済み。既存生成経路は 16-bit PCM WAV と確認済み。WAV 計算コストは未実測。

## Traceability Summary

| Claim | Requirement | Specification | Test / Review |
| --- | --- | --- | --- |
| 実音声の音圧で visualizer を変化させたい | FR-1, FR-2 | Option A / Option B | BGM/Talk/silence 手動確認 |
| 再生経路を壊したくない | NFR-1 | Option A recommended | skip/volume slider 手動確認 |
| FFT は不要 | Out of Scope | RMS scalar | build + visual QA |

## Open Questions

- envelope 計算は RegisterFile 時か lazy endpoint 初回アクセス時か。
- loudness scale の初期係数を `amp + 0.55 * rms * level` で十分とするか、手動確認で調整するか。

## Go / No-Go

Go 条件: ユーザーがこの実装順序を承認すること。WAV 対応範囲は既存生成形式である 16-bit PCM に限定して進められる。
