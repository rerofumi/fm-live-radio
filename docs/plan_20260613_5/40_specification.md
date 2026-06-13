# Implementation Specification

## Technical Stack

- React 18 + TypeScript。
- CSS modules ではなく既存の `App.css` / `style.css` を継続。
- Canvas visualizer は既存 `frontend/src/Visualizer.tsx` を継続。
- 画像生成は built-in `image_gen` を優先し、project-bound asset として保存する。

## File Structure

- `frontend/src/App.tsx`
  - メイン画面 JSX をラジオ筐体レイアウトへ再構成する。
  - `KnobControl`, `StatusLamp`, `GenreTuner` のような小さなローカル関数コンポーネントを同ファイル内に追加してよい。
- `frontend/src/App.css`
  - ラジオ筐体、木目、金属パネル、波形窓、ノブ、チューニングメーター、LED のスタイルを定義する。
- `frontend/src/style.css`
  - ラジオ風テーマの CSS 変数へ更新する。
- `main.go`
  - Wails `options.App` の `Width`, `Height`, `MinWidth`, `MinHeight` をラジオ UI に合わせて調整する。
- `frontend/src/assets/images/`
  - gpt-image 生成の texture / panel asset を使用する場合のみ追加する。
- `docs/specification.md`, `docs/requirement.md`
  - 実装完了後、現行仕様として UI 記述を反映する。

## Data Model

変更なし。`AppConfig`, `PlayableItem`, `AppStatus`, `LoudnessEnvelope` は現行のまま使う。

## UI / Screens

### Main Shell

- `.radioCabinet` を最外装に置き、木目調背景、丸みのある金属外枠、内側 shadow を持たせる。
- 上段は brand と Settings。
  - brand: radio mark + `fm-live-radio` + `AI ローカルラジオ`。
  - Settings: 右上の丸型または金属ボタン。
- 中段は `.radioDeck` として 2 カラム。
  - 左: visualizer panel + transport/mixer/status。
  - 右: tuning dial。
- 下段は現在の音楽名を銘板風に表示する。

### Window Size / Resize

- `main.go` の起動時 window は `Width: 1280`, `Height: 860` を目安に設定する。
  - 理由: 参照画像に近い横長 2 カラム UI、右側チューニングダイヤル、左側 visualizer/mixer を初期表示で収めるため。
  - 実装時にスクリーンショット確認で過大なら `1200x820` まで下げる余地を残す。
- `MinWidth: 900`, `MinHeight: 680` を目安に設定する。
  - 理由: これより下は円形チューニングメーターとツマミ群の可読性が落ちるため。
  - 最小サイズ以下のスマホ級 viewport は Wails window としては想定せず、Vite/browser 確認時のみ縦積み layout が破綻しないことを確認する。
- CSS は以下の responsive policy に従う。
  - `radioCabinet` は `width: min(100%, 1240px)` 相当で中央寄せし、広幅では筐体幅を固定気味に保つ。
  - `radioDeck` は標準では `grid-template-columns: minmax(0, 1.1fr) minmax(360px, 0.9fr)` に近い 2 カラム。
  - `max-width: 980px` 前後で 1 カラムへ切り替え、順序は header、wave/transport/mixer/status、tuning、nameplate とする。
  - 縦方向は `min-height` 固定に頼りすぎず、window が低い場合は body 側の縦スクロールを許容する。
  - 横方向は `overflow-x: hidden` を基本とし、固定幅のノブ/メーターは `clamp()` / `min()` で縮小する。
- リサイズ時に `Visualizer` は既存 `ResizeObserver` により canvas サイズへ追従するため、追加 JS は不要。

### Visualizer Panel

- 既存 `Visualizer` を `.waveDisplay` 内に配置する。
- 波形窓は黒ガラス/アクリル風にし、現在時間を右上に表示する。
- Visualizer の描画自体は変更しない。

### Transport

- Play/Pause は大きめの円形 illuminated button。
- Skip は小さい金属 button。
- disabled 状態を視覚的に弱める。

### Knob Controls

- BGM/Talk volume は `input type="range"` を維持する。
- 表示は円形ノブ、上部 pointer、周辺 tick/arc、現在値パーセントで構成する。
- `style={{"--knob-angle": "...deg"}}` のような CSS custom property を使い、値に応じて pointer を回転させる。
- input は視覚的に隠しすぎず、キーボード操作可能な透明 overlay または下部 range として保持する。

### Status Lamps

- Talk と Music の各行に以下の状態を表示する。
  - generating: amber/blue LED pulse。
  - ready: green LED。
  - idle: dim LED。
  - local error: red LED。
- 既存 `talkPrefetching`, `talkReady`, `musicGenerating`, `musicReady`, `errorText` を利用する。

### Tuning Dial

- 右側に円形 meter を配置する。
- 上部に ON AIR LED と label。
- 中央に `nowTitle` / `nowSub` を表示する。
- `SA3_GENRES` を円弧上またはスケール上の選択肢として表示する。
- 操作は hidden/native select ではなく、各 genre button を押す形にする。選択時は現行と同じ `UpdateStableAudio3Genre` を呼ぶ。
- 選択 genre に応じて needle 角度を変える。

### Settings Modal

- 既存機能を維持する。
- モーダルはメインテーマに合わせて色を調整してよいが、設定項目の削除はしない。

## Inputs

- Play/Pause button: `onPlayPause`。
- Skip button: `onSkip`。
- BGM/Talk range: `persistConfig`。
- Genre tuner button/select fallback: `UpdateStableAudio3Genre`。
- Settings: `setShowSettings(true)`。

## Persistence

現行どおり `SaveConfig` と `UpdateStableAudio3Genre` を使う。

## Error Handling

- 既存 `errorText` は赤 LED と警告領域に表示する。
- `UpdateStableAudio3Genre` 失敗時は現行と同じく `setErrorText`。
- 画像アセットがなくても CSS gradient で最低限の木目/金属表現が出るようにする。

## Import / Export

なし。

## Environment Constraints

- コマンドは `mise` 経由。
- VCS 操作は `.jj` があるため `jj` を使う。
- 画像生成は built-in `image_gen` を使い、プロジェクトで使う場合は生成後に `frontend/src/assets/images/` へ移す。
- Wails window size は `main.go` の `options.App` で指定し、runtime resize handler は追加しない。

## Verification

1. `mise x -- npm --prefix frontend run build`
2. 可能なら `mise run dev` または `mise x -- npm --prefix frontend run dev -- --host 127.0.0.1` で表示確認。
3. ブラウザ/アプリ目視:
   - デスクトップ幅で 2 カラム表示。
   - 狭幅で縦積み表示。
   - Wails 起動時に標準 window size で主要 UI が見切れない。
   - window resize 時に横スクロールや操作不能な重なりが発生しない。
   - Visualizer が表示される。
   - Settings ボタンが右上にある。
   - genre を変更して UI の needle / active state が変わる。
