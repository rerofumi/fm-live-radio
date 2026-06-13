# Review Checklist

## Security

- [ ] loudness endpoint は token なしでファイル情報を漏らさない。
- [ ] CORS ヘッダの許可範囲が local app 用として妥当。

## Frontend

- [ ] Visualizer の既存 mode / kind / level 表現を壊していない。
- [ ] reduced-motion 時に連続 animation を再開しない。
- [ ] fetch 失敗を toast に出さず静かに fallback する。
- [ ] 描画 loop は React の `elapsedSec` ではなく `<audio>.currentTime` で envelope を参照している。
- [ ] item 切替時に古い envelope を即破棄し、古い fetch response を採用しない。

## Backend

- [ ] WAV 形式の対応範囲は 16-bit PCM として明文化されている。
- [ ] token 期限切れ時に envelope cache も消える。
- [ ] RegisterFile で非 WAV を受けても再生は壊れない。
- [ ] envelope 生成失敗が audio URL 発行を失敗させない。

## DB / Storage

- [ ] 永続 storage 変更がない。

## QA / Test

- [ ] BGM、Talk、silence、skip、volume slider の手動確認がある。
- [ ] BGM と Talk で音の山と visualizer の山に目立つ遅延がない。
- [ ] `mise x -- go test ./...` が通る。
- [ ] `mise x -- npm run build` 相当が通る。

## DevOps / Environment

- [ ] `mise.toml` の task / tool 方針を守る。
- [ ] Wails dev と build の両方で CORS / endpoint 到達性を確認する。

## Pre-Implementation Research

- [ ] W3C Web Audio CORS silence の仕様根拠を確認済み。
- [ ] MDN AnalyserNode / time-domain data の挙動を確認済み。
- [ ] `captureStream()` の Limited availability を確認済み。

## Traceability

- [ ] Claim、Requirement、Specification、Test / Review に未接続の要求がない。
