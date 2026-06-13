# Claim

## Purpose

現在の visualizer は再生状態と音量設定だけでそれらしく動いている。実際に再生される音声の音圧、少なくとも RMS 相当のラウドネスに合わせて波形の振幅や発光を変化させたい。

## Background

`frontend/src/App.tsx` は単一の `<audio>` 要素へ Go audio server の token URL を設定して再生している。`frontend/src/Visualizer.tsx` は Canvas 2D と `requestAnimationFrame` で合成波形を描き、`playing` / `kind` / `level` のみを入力にしている。

既存 `docs/cheatsheet/frontend-visualizer.md` には、CORS なしのクロスオリジン音源を Web Audio へ接続すると解析データが無音化しうるため、実 FFT を避けた経緯がある。

## Problem

ユーザーの期待はスペクトラムアナライザではなく、音圧に合わせて「ぴょこぴょこ」変化すること。現状は音楽の強弱や無音に反応できない。

## Target Users / Environment

- Wails desktop app 利用者。
- 主対象は Windows / WebView2。
- 音源は Stable Audio 3 BGM WAV と IrodoriTTS Talk WAV。

## Initial Scope

- 実装前の原理調査。
- Web Audio API、CORS、Wails ローカル audio server、バックエンド envelope 計算の比較。
- この計画ではコード変更しない。

## Initial Technical Hypotheses

- Web Audio `AnalyserNode.getByteTimeDomainData()` または `getFloatTimeDomainData()` で FFT なしの RMS は計算できる。
- ただし `<audio>` の URL はフロントエンド origin と異なるため、Web Audio 解析には `crossOrigin="anonymous"` と audio server の CORS ヘッダが必要になる。
- 既存の再生経路を壊しにくい案は、Go 側で WAV から短い時間窓の RMS/peak envelope を事前計算し、フロントが再生時刻で参照する方式。

## Uncertainties

- Go 側で生成 WAV の形式が常に PCM 16-bit とみなせるか。
- envelope を `RegisterFile` 時に計算しても UX に影響しないか。
- Web Audio 方式を採る場合、React StrictMode や再生 item 切替時に `MediaElementAudioSourceNode` を重複生成しない設計が必要。

## Next Documents

`20_app_requirement.md`、`30_requirement.md`、`40_specification.md`、`50_review_notes.md`、reviewer 向け文書、および `docs/cheatsheet/frontend-visualizer.md` へ検証済み知見を反映する。
