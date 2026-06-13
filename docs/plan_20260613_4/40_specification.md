# Implementation Specification

## Technical Stack

- Frontend: React, TypeScript, Canvas 2D。
- Backend: Go local audio server。
- Primary approach: WAV envelope precompute。
- Alternative approach: Web Audio `MediaElementAudioSourceNode` + `AnalyserNode` time-domain RMS with explicit CORS.

## Research Summary

### Confirmed Facts

- `AnalyserNode` は周波数および時間領域データを取得でき、音声を変更せずに visualization 用データを取れる。
- `getByteTimeDomainData()` は現在の waveform/time-domain データを `Uint8Array` へコピーする。RMS はこの値を中心 128 からの差分として二乗平均平方根にすればよい。
- `createMediaElementSource()` は `<audio>` / `<video>` を Web Audio graph に入れる API で、呼び出すと media element の音声再生は AudioContext graph に reroute される。
- W3C Web Audio 仕様は、CORS-cross-origin と判定された media element 由来の `MediaElementAudioSourceNode` は通常出力ではなく silence を出すべきとしている。
- `HTMLMediaElement.crossOrigin` 未指定時、media resource は `no-cors` で取得される。
- `captureStream()` は media element の再生内容を `MediaStream` として返すが、MDN では Limited availability。

### Local Observations

- `internal/audio/server.go` は `http://127.0.0.1:<dynamic>/audio/<token>` で `http.ServeFile` を使い、CORS ヘッダを付与していない。
- `App.tsx` は単一 `<audio ref={audioRef}>` に `item.url` を設定して再生している。
- `Visualizer.tsx` は音声要素を受け取らず、`playing` / `kind` / `level` のみで描画する。

## Option A: Backend WAV Envelope (Recommended / Approved Direction)

### Design

1. `RegisterFile(path, ttl)` で token を発行するとき、対象が WAV なら短い window ごとの RMS/peak envelope を計算して token に紐付ける。
2. audio server に `/loudness/<token>` の JSON endpoint を追加し、envelope 全体を返す。
3. `PlayableItem` に `loudnessUrl` を追加し、audio URL と同じ token に対応する envelope URL を frontend へ渡す。
4. frontend は item 切替時に envelope を一度だけ取得し、描画時は `HTMLAudioElement.currentTime` からローカル配列を参照する。
5. `Visualizer` は参照した loudness 値を `amp` / `energy` に混ぜる。
6. 失敗時は loudness なしの既存 animation を使う。

### API Shape

```json
{"windowMs":50,"sampleRate":44100,"durationSec":30.0,"rms":[0.04,0.12,0.20],"peak":[0.08,0.30,0.55]}
```

MVP では `rms` のみ必須、`peak` は利用可能なら返す。`rms[i]` は `i * windowMs` から `(i + 1) * windowMs` の音声窓を表す。

### Pros

- `<audio>` の再生経路を変更しない。
- Web Audio の CORS silence 問題を避けられる。
- WAV 生成済みファイルという既存制約と相性がよい。
- Windows 以外でも同じ考え方で動く。

### Cons

- 実際の再生音そのものではなくファイル内容の envelope。volume slider は frontend 側で掛け合わせる必要がある。
- WAV decoder の対応範囲を決める必要がある。
- 初回計算コストを測る必要がある。
- WAV 先頭からの envelope なので、ブラウザ decode delay や `currentTime` 更新粒度による小さな視覚ズレは残る。ただし polling 方式よりズレは小さい。

## Option B: Web Audio Time-Domain RMS

### Design

1. `<audio crossOrigin="anonymous">` を `src` 設定前に用意する。
2. audio server が `Access-Control-Allow-Origin` を返す。
3. `AudioContext.createMediaElementSource(audio)` で source を作り、`source -> analyser -> destination` に接続する。
4. `getFloatTimeDomainData()` または `getByteTimeDomainData()` から RMS を計算する。

### Pros

- 実際に decode されている audio stream に近い値を取れる。
- RMS は FFT 不要。

### Cons

- CORS 設定が必須。
- AudioContext lifecycle、React StrictMode、item 切替での重複 source 作成に注意が必要。
- media element の音声経路が AudioContext graph に変わるため、再生安定性への影響が Option A より大きい。

## Option C: HTMLMediaElement.captureStream

`captureStream()` で `MediaStream` を取得して `MediaStreamAudioSourceNode` へ接続する案。MDN で Limited availability のため、MVP では採用しない。

## File Structure

- `internal/audio/server.go`: envelope cache、loudness URL 発行 helper、`/loudness/<token>` endpoint を追加する。
- `internal/audiofmt/wav.go`: 既存 `DecodeWavPCM16` を使い、PCM16 WAV から envelope を計算する helper を追加する。
- `internal/domain/types.go`: `PlayableItem` に `LoudnessURL string 'json:"loudnessUrl,omitempty"'` を追加する。
- `internal/player/player.go`: `audioSrv.RegisterFile(...)` 呼び出し箇所で `LoudnessURL` も item に詰める。
- `frontend/src/App.tsx`: `PlayableItem` type に `loudnessUrl?: string` を追加し、item 切替時に envelope を fetch して state に保持する。`audioRef.current` と envelope を Visualizer に渡す。
- `frontend/src/Visualizer.tsx`: `audio?: HTMLAudioElement | null`、`loudness?: LoudnessEnvelope | null` を受け取り、描画 loop 内で `audio.currentTime` から loudness を参照する。
- `frontend/wailsjs/go/models.ts`: Wails binding 再生成対象。手動編集ではなく `wails generate module` または build 時の生成に従う。

## Data Model

```go
type loudnessEnvelope struct {
    WindowMS int
    Values []float64
    Peaks []float64
}
```

MVP は 50ms window、0..1 正規化 RMS を想定する。50ms は音の山への追従と JSON サイズのバランスがよく、30 秒音源でも約 600 点に収まる。

Go response:

```go
type LoudnessEnvelopeResponse struct {
    WindowMS    int       `json:"windowMs"`
    SampleRate  int       `json:"sampleRate"`
    DurationSec float64   `json:"durationSec"`
    RMS         []float64 `json:"rms"`
    Peak        []float64 `json:"peak,omitempty"`
}
```

Frontend type:

```ts
type LoudnessEnvelope = {
  windowMs: number;
  sampleRate?: number;
  durationSec?: number;
  rms: number[];
  peak?: number[];
};
```

RMS 計算:

1. `audiofmt.DecodeWavPCM16` で WAV を PCM16LE、sample rate、channels に分解する。
2. frame ごとに全 channel の二乗値を平均する。
3. 50ms 窓ごとに `sqrt(sum(sample^2)/count) / 32768` を `rms` とする。
4. `peak` は同じ窓の最大絶対値 `/ 32768` とする。
5. 値は `[0, 1]` に clamp する。

Visualizer 参照:

```ts
const index = Math.floor(audio.currentTime * 1000 / envelope.windowMs);
const raw = envelope.rms[Math.max(0, Math.min(envelope.rms.length - 1, index))] ?? 0;
const audible = raw * level;
```

`audible` は急峻すぎると見た目が荒れるため、既存の frame-rate independent smoothing の target 値に混ぜる。推奨初期値は `amp += audible * 0.55`、`energy += audible * 0.35`。

## Implementation Steps

1. `audiofmt` に PCM16 envelope helper と unit test を追加する。
2. `audio.Server` に token ごとの envelope cache と `/loudness/<token>` endpoint を追加する。
3. `domain.PlayableItem` に `loudnessUrl` を追加し、player が BGM/Talk item 作成時に設定する。
4. `App.tsx` で item 切替時に `loudnessUrl` を fetch し、失敗時は `null` にする。skip / next item 時は先に `null` へリセットする。
5. `Visualizer.tsx` が `audioRef.current` と envelope を使って currentTime 基準で loudness を読む。
6. frontend build、Go test、手動再生確認を行う。

## Drift Mitigation

- envelope は network polling ではなく item 開始前後にまとめて取得する。
- 描画 loop は React state の `elapsedSec` ではなく、実際の `<audio>.currentTime` を読む。
- item 切替時は envelope state を即 `null` にし、fetch 完了後に新 item ID と一致する場合だけ採用する。
- 50ms window と smoothing により、WAV 先頭基準とブラウザ decode 表示の小さなズレを目視上吸収する。
- 将来ズレが目立つ場合は `visualizerLoudnessOffsetMs` のような内部定数を追加して、`currentTime + offset` で補正できる余地を残す。

## Error Handling

- 非 WAV、decode 失敗、期限切れ token は `404` または `204` を返し、frontend は既存合成 animation に fallback。
- fetch 失敗は toast 表示しない。
- envelope 生成失敗は server log に留め、`RegisterFile` 自体の audio URL 発行は失敗させない。
- loudness response が巨大または壊れている場合、frontend は配列長と `windowMs` を検証し、無効なら破棄する。

## Environment Constraints

- audio server はフロントエンド origin と異なる可能性が高いため、JSON endpoint には CORS ヘッダを付ける。
- Web Audio 方式を採る場合は CORS ヘッダだけでなく `<audio>.crossOrigin` を `src` 設定前に指定する。

## Verification

- `mise x -- npm run build`
- `mise x -- go test ./...`
- 手動: `mise x -- wails dev` で再生、BGM/Talk/silence/skip/volume slider を確認。
- 手動: 音の山に対して visualizer の山が大きく遅れないことを BGM と Talk で確認する。
