# Requirements

## Scope

対象はメイン UI の見た目と操作配置の刷新。React コンポーネント構造と CSS を中心に変更し、必要に応じて project-bound 画像アセットを追加する。

## Functional Requirements

- Play/Pause、Skip、BGM volume、Talk volume、SA3 Genre、Settings は現行と同じ操作結果を維持する。
- Settings ボタンはメイン画面右上に常時表示する。
- BGM/Talk volume はツマミ風 UI として表示し、値の変更が即時保存される。
- SA3 Genre はチューニングメーター/ダイヤル風 UI として表示し、変更時に `UpdateStableAudio3Genre` を呼ぶ。
- Visualizer は既存 `Visualizer` コンポーネントを使い、波形窓内で表示する。
- ON AIR / OFF AIR と current kind はチューニングメーター内またはその近傍で確認できる。
- Talk/Music の生成中・先読み完了状態は LED 風に表示する。
- `localGenerationError` または UI error は Local error LED と toast/警告表示に反映する。
- 画面幅が狭い場合は、左の操作盤と右のチューニングメーターが縦積みになり、操作不能にならない。
- 起動時 Wails window はラジオ筐体 UI の標準表示に合わせ、現行 `1024x768` から横長の初期サイズへ変更する。
- Wails window には最小サイズを設定し、UI が破綻する極端な縮小を抑止する。

## Non-Functional Requirements

- 操作可能要素は button / input / select 等のネイティブセマンティクスを維持する。
- テキストがボタンやパネルからはみ出さない。
- 色は木目・金属・アンバー・赤/緑 LED を中心にしつつ、単一色相だけに偏らせない。
- `prefers-reduced-motion` 時は既存 Visualizer と LED pulse の過度なアニメーションを抑える。
- 画像アセットを使う場合でも、表示失敗時に主要操作が読めなくならない。
- リサイズ時は CSS grid/flex と breakpoint で応答し、JavaScript による手動レイアウト計算に依存しない。
- 広幅では最大表示幅を設け、筐体が過度に横伸びしない。
- 狭幅では縦スクロールを許容し、横スクロールを前提にしない。

## Constraints

- 既存の Wails generated API と backend model は変更しない。
- `frontend/package.json` へ新規 UI ライブラリを追加しない。
- アイコンは既存依存にアイコンライブラリがないため、CSS/テキスト記号で最小実装する。
- 重要なラベルや状態表示を画像内テキストだけに依存しない。
- gpt-image 生成物はプロジェクトで参照する場合、`frontend/src/assets/images/` に保存する。

## Compatibility

- `mise x -- npm --prefix frontend run build` が成功する。
- Wails build に影響しない。
- `main.go` の Wails window option は Wails v2 の `options.App` 標準フィールドに収める。
- 既存 Settings モーダルで全設定を保存できる。

## Out of Scope

- Go backend の変更。
- Settings モーダル全体の機能拡張。
- 音声ビジュアライザのアルゴリズム変更。
- 参照画像の日本語注釈やウィンドウ枠の再現。

## Acceptance Criteria

- メイン画面がラジオ筐体として視認できる。
- 起動時 window が横長 UI を見切れなく表示できる。
- window を狭めたとき、2 カラムから縦積みに変化し、主要操作が利用できる。
- window を広げたとき、UI が中央にまとまり、可読性が維持される。
- ツマミ、パイロット LED、ジャンル選択チューニング、波形窓、右上 Settings が存在する。
- Play/Pause、Skip、BGM/Talk volume、Genre、Settings の既存操作が動く。
- `npm --prefix frontend run build` が成功する。
- 可能であれば Wails dev または Vite preview でデスクトップ幅と狭幅の表示を目視確認する。
