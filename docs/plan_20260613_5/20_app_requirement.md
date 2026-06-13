# App Requirements

## Purpose

`fm-live-radio` のメイン画面を、AI ローカルラジオらしい物理ラジオ風の操作盤へ刷新する。既存の再生・設定・生成状態確認の操作は維持する。

## Target Environment

- Wails v2 + React + TypeScript + Vite。
- Windows WebView2 を主な表示環境とする。
- 起動時 window は参照画像に近い横長 UI が収まるサイズで開く。
- window resize に対応し、狭幅でも主要操作が画面内に残る。
- 開発・検証は `mise` 経由で行う。

## User Experience

- 画面を開くと、木目筐体と金属パネルを持つラジオ機器として見える。
- 左側に波形ディスプレイ、再生ボタン、Skip、BGM/Talk ツマミ、生成状態 LED を配置する。
- 右側に円形チューニングメーターを配置し、現在の BGM provider / genre / title と ON AIR 状態を表示する。
- ジャンル選択は select そのものではなく、チューニングダイヤルまたはチューニングスケールとして見える。
- Settings は右上からいつでも開ける。
- エラーは機器下部または上部に目立ちすぎない警告として表示する。
- ユーザーが window を広げた場合はラジオ筐体が中央に配置され、余白が不自然に伸びすぎない。
- ユーザーが window を狭めた場合は、左右 2 カラムを縦積みに切り替え、波形、操作、チューニングの順で利用できる。

## MVP Features

- 参照画像から以下を取り込む。
  - 木目のラジオ筐体。
  - brushed metal 風のパネル。
  - 左側の波形表示窓。
  - 円形の再生ボタンと Skip ボタン。
  - BGM Volume / Talk Volume のツマミ表現。
  - Talk / Music status の生成中・先読み完了・エラーを LED 表現で表示。
  - 右側の円形チューニングメーター。
  - Genre/Tuning 操作。
  - 右上 Settings ボタン。
- 既存 Visualizer を継続利用する。
- 既存の設定保存・ジャンル即時更新・再生制御の挙動を変えない。
- 起動時 Wails window サイズと最小 window サイズを UI に合わせて設定する。

## Future Candidates

- 生成画像ベースの高精細ノブ/ベゼルを個別アセット化する。
- チューニング針のアニメーションを、ジャンル変更時に短く振れるようにする。
- CSS Houdini や Canvas を使ったより細密な金属質感。
- Settings モーダルもラジオ機器風に再設計する。

## Initial Non-Goals

- バックエンド API の変更。
- 音声生成、再生順序、loudness envelope の再設計。
- 参照画像の完全コピー。
- OS ウィンドウ枠のカスタム描画。
- 画像内の欠けた Settings ボタンの欠けまで再現すること。
