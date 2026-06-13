# Claim

## Purpose

現行のミニマルな UI を、参照画像のような「卓上ラジオ機器」を前面に出した UI に刷新する。ラジオのメタファーを使い、再生体験、状態表示、ジャンル選択を直感的に見せる。

## Background

ユーザー提供の参照画像には、木目の筐体、金属パネル、円形チューニングメーター、パイロット LED、物理ツマミ、波形表示、再生/スキップボタン、ジャンルチューニングが含まれている。完全コピーではなく、主要要素を拾い上げて既存アプリの機能に合わせて実装する。

## Problem

現行 UI は機能的だが、ラジオアプリとしての手触りやメタファーが弱い。特にジャンル選択、生成/先読み状態、再生中の BGM 情報を一体的に見せる余地がある。

## Target Users / Environment

- Windows x64 の Wails デスクトップアプリ利用者。
- ローカル AI ラジオを「機器を操作している」感覚で使いたいユーザー。
- マウス操作とキーボードフォーカスの両方で操作できることを前提にする。

## Initial Scope

- メイン画面のレイアウトとスタイル刷新。
- 起動時の Wails window サイズを、横長ラジオ筐体 UI に合わせて調整する。
- アプリ window のリサイズ時に、操作不能・表示欠け・テキスト重なりが起きない responsive behavior を定義する。
- 既存 Visualizer の Canvas 実装は維持し、ラジオ筐体内の波形窓として配置する。
- BGM/Talk 音量は物理ツマミ風コントロールに見せる。
- Stable Audio 3 genre はチューニングダイヤル風 UI で選択する。
- Settings ボタンは右上に追加する。
- gpt-image は背景/質感パーツ作成に使えるが、操作可能 UI の重要部分は HTML/CSS で実装する。

## Initial Technical Hypotheses

- 主要変更は `frontend/src/App.tsx`, `frontend/src/App.css`, `frontend/src/style.css` に閉じられる。
- 起動時 window サイズと最小サイズは `main.go` の Wails `options.App` (`Width`, `Height`, `MinWidth`, `MinHeight`) で指定できる。
- 既存 Wails API、再生制御、状態 polling、loudness envelope 取得は変更不要。
- 画像アセットは `frontend/src/assets/images/` 配下に保存し、CSS background として参照できる。
- 物理ツマミは `<input type="range">` を維持しつつ、CSS と `style` 変数で回転角を表現できる。

## Uncertainties

- gpt-image 生成の質感アセットを使うか、CSS の wood/metal texture で十分かは実装前に最小試作で判断する。
- 参照画像の曲線的な波形窓を完全再現するには CSS clip-path が必要になる可能性がある。MVP ではレスポンシブ安定性を優先し、近い形状に留める。
- Settings モーダルの内部レイアウトは既存機能維持を優先し、全面的な再設計は対象外にする。

## Next Documents

1. `20_app_requirement.md`
2. `30_requirement.md`
3. `40_specification.md`
4. `50_review_notes.md`
5. `60_review_brief.md`
6. `70_review_board.html`
7. `80_review_checklist.md`
