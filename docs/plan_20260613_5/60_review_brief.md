# Review Brief

## Review Purpose

参照画像に基づくラジオ機器風 UI 刷新の実装前レビュー。

## Review Scope

- メイン画面の React/CSS レイアウト。
- 既存操作の維持。
- gpt-image 生成アセットの利用方針。
- レスポンシブとアクセシビリティの最低限の維持。

## Decisions Needed

- この計画どおり、コードネイティブ UI を主として実装してよいか。
- gpt-image はまず texture/質感補助に使い、操作 UI は CSS/HTML で構築する方針でよいか。
- Settings モーダルの本格再設計を今回対象外にしてよいか。
- 起動時 window を `1280x860` 目安、最小 window を `900x680` 目安にする方針でよいか。

## Maximum Risks

- 参照画像に寄せすぎると、レスポンシブ表示や操作性が落ちる。
- CSS の質感表現が過剰になると可読性が下がる。
- 画像アセットのテキストに依存すると、状態表示が壊れやすい。
- 初期 window size が大きすぎると小型ディスプレイで扱いにくい。最小サイズが小さすぎると円形 UI が破綻する。

## Pre-Implementation Research Status

- 現行 UI 実装確認済み: `frontend/src/App.tsx`, `App.css`, `style.css`, `Visualizer.tsx`。
- 現行 docs 確認済み: `docs/requirement.md`, `docs/specification.md`。
- Visualizer の既存実装知見確認済み: `docs/cheatsheet/frontend-visualizer.md`。
- 外部 API 調査は不要。画像生成は built-in `image_gen` を利用する。

## Traceability Summary

| Claim | Requirement | Specification | Test / Review |
| --- | --- | --- | --- |
| ラジオ機器風 UI に刷新 | 木目筐体、金属、波形窓、LED、ツマミ、チューニング | Main Shell / Visualizer Panel / Knob / Status Lamps / Tuning Dial | 目視 QA、frontend build |
| 既存操作を維持 | Play/Skip/Volume/Genre/Settings が同じ結果 | Inputs / Persistence | 手動操作確認 |
| Settings は右上 | Settings 常時表示 | Main Shell | 目視 QA |
| Window sizing | 起動時サイズ、最小サイズ、リサイズ時挙動 | Window Size / Resize | Wails dev 目視 QA |

## Open Questions

- なし。見た目の細部は、実装時のスクリーンショット確認で調整する。

## Go / No-Go

- Go 条件: 本計画へのユーザー承認。
- No-Go 条件: 参照画像の完全再現や Settings モーダル全面刷新も同時に要求される場合は、計画を更新する。
