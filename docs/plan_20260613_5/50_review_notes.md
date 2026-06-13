# Review Notes

## Review Findings

- `frontend/src/App.tsx`, `frontend/src/App.css`, `frontend/src/style.css`, `main.go` を計画どおり更新済み。
- `mise x -- npm --prefix frontend run build` は成功。
- Vite ブラウザ確認では Wails API が存在しないため設定読み込みエラー toast が出るが、ラジオ筐体・2 カラム・縦積み・横スクロールなしのレイアウトは確認済み。
- 追加確認で、チューナーの半円スケール・針・ボタンの配置ズレを修正。針角度は 2x2 ボタン配置の左右位置に対応する明示マッピングへ変更済み。

## Fixed Items

- メイン UI を木目筐体、金属パネル、波形窓、ノブ、チューニングメーター、LED 状態表示へ刷新。
- Settings ボタンを右上に常時配置。
- チューニングメーター内の `tunerDial` 領域を追加し、目盛と針の支点を同じ座標系へ揃えた。
- Wails window を `1280x860`, minimum `900x680` に調整。
- 現行 docs (`docs/requirement.md`, `docs/specification.md`) に実装後の UI 仕様を反映。

## Deferred Items

- Settings モーダル全体のラジオ風再設計。
- 画像生成ノブ/ベゼルの個別アセット化。

## Rejected Options

- 参照画像を一枚背景として貼り、その上に透明ボタンを置く方式。
  - 理由: レスポンシブ性、アクセシビリティ、状態更新、文字差し替えに弱い。
- 新規 UI ライブラリ導入。
  - 理由: 今回はビジュアル刷新が主目的で、依存追加なしで実装可能。

## Current Non-Goals

- バックエンド契約の変更。
- Visualizer の描画アルゴリズム変更。
- OS ウィンドウ装飾の変更。

## Future Plans

- 実装後の目視 QA で、必要なら gpt-image 生成の texture asset を追加または差し替える。
- Settings モーダルの視覚統一を次計画で扱う。

## Next Plan Candidates

- 「ラジオ機器風 Settings モーダル刷新」。
- 「チューニング針/LED アニメーション強化」。

## Documentation Feedback

- `docs/requirement.md` と `docs/specification.md` のフロントエンド仕様を更新済み。
