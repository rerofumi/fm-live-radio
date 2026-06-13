# Requirements

## Scope

実装する場合は、既存 visualizer を実音声由来の loudness 値で変化させる。今回の調査結論では、MVP はバックエンドで WAV から RMS/peak envelope を計算する方式を第一候補とする。

## Functional Requirements

- FR-1: 現在再生中 item に対応する loudness 値を 0..1 のスカラーとして取得できる。
- FR-2: `Visualizer` は loudness 値を既存の `playing` / `kind` / `level` ベースの target に加算または乗算して描画する。
- FR-3: `bgm`、`talk`、`silence` の既存見た目差分は維持する。
- FR-4: loudness 取得に失敗した場合、既存 visualizer と同じ挙動へ戻る。
- FR-5: skip や item 切替で古い loudness 値が次 item に残らない。
- FR-6: reduced-motion 時は連続アニメーションを再開しない。
- FR-7: loudness は item 切替時に envelope 全体を取得し、描画時は `<audio>.currentTime` からローカル参照する。
- FR-8: envelope の時刻基準は WAV 先頭 0 秒とし、`<audio>.currentTime` と同じ秒単位で扱う。
- FR-9: 音量 slider の値は loudness 値へ乗算し、聴こえる音量と見た目の振幅差を小さくする。

## Non-Functional Requirements

- NFR-1: 再生開始、skip、音量 slider、`onEnded` の既存フローを壊さない。
- NFR-2: loudness 取得処理は UI thread と再生開始を目立って遅延させない。
- NFR-3: token 期限切れや 404 で console error を連発しない。
- NFR-4: 描画フレームごとの loudness 参照は network polling に依存しない。
- NFR-5: envelope 生成に失敗しても audio URL 発行と再生は成功させる。

## Constraints

- `MediaElementAudioSourceNode` 方式は、W3C Web Audio 仕様上、CORS-cross-origin と判定された媒体では silence を出力する。
- `crossOrigin` 未指定の media element fetch は `no-cors` になるため、Web Audio 解析を使う場合は要素側 `crossOrigin` と server 側 CORS ヘッダを揃える必要がある。
- `createMediaElementSource()` は media element の音声を AudioContext graph に reroute するため、採用するなら AudioContext lifecycle を慎重に扱う。

## Compatibility

- Windows / WebView2 では Web Audio API と `AnalyserNode` は利用可能と見込む。
- `HTMLMediaElement.captureStream()` は MDN で Limited availability とされるため、MVP 依存にはしない。

## Out of Scope

- FFT / spectrum analyzer。
- 音声認識、拍検出、BPM 推定。
- Web Audio Worklet。

## Acceptance Criteria

- AC-1: BGM の強弱に合わせて visualizer の振幅が目視で変化する。
- AC-2: 無音 gap で低振幅になる。
- AC-3: volume slider を下げた場合、見た目の振幅も小さくできる。
- AC-4: Skip 連打で再生と visualizer が固まらない。
- AC-5: `mise x -- npm run build` 相当の frontend build が通る。
- AC-6: 同じ音源を 0 秒から再生したとき、視覚反応の山が音の山から大きく遅れて見えない。
- AC-7: item 切替直後、前 item の envelope による大振幅が残らない。
