# App Requirements

## Purpose

Visualizer を実音声のラウドネスに反応させ、BGM や Talk の強弱が UI 上でも自然に伝わるようにする。

## Target Environment

- Wails desktop app。
- 主対象は Windows / WebView2。
- 開発環境は `mise` 経由の `go` / `wails` / `npm`。

## User Experience

- BGM 再生中、音の山で波形が大きくなり、静かな部分で落ち着く。
- Talk 再生中も声の抑揚に合わせて振幅が変化する。
- 無音 gap では低振幅になる。
- 既存の mode / kind ごとの色や雰囲気は維持する。

## MVP Features

- 実音声からスカラーの loudness 値を得る。
- loudness 値を既存 `Visualizer` の `amp` / `energy` に混ぜる。
- loudness 取得に失敗した場合は既存の合成アニメーションへ fallback する。
- スペクトラム表示、周波数帯別バー、FFT UI は実装しない。

## Future Candidates

- Web Audio `AnalyserNode` によるリアルタイム RMS。
- 周波数帯別の軽量 visual effect。
- envelope のキャッシュ共有や WAV 形式対応拡張。

## Initial Non-Goals

- 音声再生エンジンの置き換え。
- `<audio>` を Web Audio graph 前提の再生に全面移行すること。
- LUFS や BS.1770 相当の厳密なラウドネス計測。
