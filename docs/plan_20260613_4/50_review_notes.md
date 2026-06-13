# Review Notes

## Review Findings

- 外部調査として `opencode` を実行し、比較案を収集した。
- `agy` も実行したが、正常終了したものの出力が空だった。
- 重要主張は Codex 側で W3C / MDN / Wails 公式情報を再確認した。

## Fixed Items

- 外部エージェントが誤って `cmd/local_smoketest/main.go` を変更したため、調査範囲外として元に戻した。
- 外部エージェントの raw research には「Wails WebView2 なら CORS 不要に近い」という未検証の推論が含まれていたため、検証済み計画では「audio server は別 origin とみなし CORS を明示する」方針へ修正した。

## Deferred Items

- WAV envelope 計算コストのローカル実測。
- Web Audio 方式の実機検証。
- 視覚的な loudness scale と smoothing 係数の微調整。

## Rejected Options

- `captureStream()` は Limited availability のため MVP 依存にしない。
- Audio Worklet は要件に対して過剰。

## Current Non-Goals

- FFT / spectrum analyzer。
- LUFS 相当の厳密 loudness。

## Future Plans

- 実装済みの Option A を実機再生で確認し、必要なら loudness scale / smoothing 係数のみ微調整する。
- as-built は現行 `docs/specification.md` と `docs/requirement.md` へ反映済み。

## Plan Update 2026-06-13

- ユーザー懸念: 「別計算の音圧データで表示をコントロールすると曲とのズレが起きないか」。
- 対応方針: per-frame fetch ではなく envelope 全体を item 切替時に取得し、描画 loop では `<audio>.currentTime` を直接参照する。
- 既存コード確認: Stable Audio 3 BGM と IrodoriTTS Talk はどちらも 16-bit PCM WAV を書き出しているため、MVP は `audiofmt.DecodeWavPCM16` を再利用する。

## Documentation Feedback

- `docs/cheatsheet/frontend-visualizer.md` の「実 FFT を使わない」記述に、今回の「RMS envelope なら採用可能」という更新を加えた。

## Implementation Notes 2026-06-13

- 実装方針は計画通り Option A のみ。Web Audio / FFT / `captureStream` / `AudioContext` は導入していない。
- 追加コンポーネント:
  - `internal/audiofmt/wav.go` に `LoudnessEnvelope` 型と `ComputeWavLoudnessEnvelope` / `ComputeWavLoudnessEnvelopeFile` を追加。
  - `internal/audiofmt/wav_test.go` に silence、フルスケール、windowing、トレーリング部分窓、ステレオ平均、非 WAV、デフォルト窓のテストを追加。
  - `internal/audio/server.go` に envelope cache、`/loudness/<token>` ハンドラ、CORS、`LoudnessURLForAudioURL`、token GC 連動の envelope 破棄を追加。
  - `internal/domain/types.go::PlayableItem` に `LoudnessURL` (`json:"loudnessUrl,omitempty"`) を追加。
  - `internal/player/player.go` の BGM / Talk(prefetch + 直接生成) 3 経路すべてで `audioSrv.LoudnessURLForAudioURL(url)` を `PlayableItem.LoudnessURL` に設定。
  - `frontend/src/Visualizer.tsx` に `audio` / `loudness` props を追加し、描画 loop 内で `audio.currentTime` と `envelope.windowMs` から RMS を引いて `amp += raw * level * 0.55`、`energy += raw * level * 0.35` を上限 clamp 付きで混入。`playing=false`、`kind='silence'`、`audio.paused`、envelope 不在のいずれかでは混入をスキップ。
  - `frontend/src/App.tsx` で `loudness` state、`current?.id` 変更時の即時 reset、`AbortController` + `currentIdRef` による stale fetch 破棄、204 / 404 / parse 失敗時の silent fallback、`stopPlayback` での reset を実装。受信値は `[0, 1]` に再 clamp。
  - `frontend/wailsjs/go/models.ts::PlayableItem` を手動更新（Wails CLI を再生成していないため）。
- envelope の analysis window は仕様通り 50 ms 固定（`internal/audio/server.go::loudnessWindowMs`）。30 秒 BGM で 600 点、JSON で概ね 10 KB 弱に収まる想定。
- 検証:
  - `mise x -- go test ./...` 全パス（既存 musicgen / store / 新規 audiofmt）。
  - `frontend/` 配下で `mise x -- npm run build` がエラーなく成功。
  - `mise x -- go build ./...` がエラーなく成功。
- 既知の制限 / 受け入れ済みトレードオフ:
  - precompute は同期。Stable Audio 3 / IrodoriTTS が出す 30 秒前後の 16-bit PCM WAV では十分速いが、極端に長い WAV では `RegisterFile` の応答が遅くなる可能性がある。今回の用途では問題視しない。
  - `RegisterFile` 経路では envelope cache を取り違えないよう、`/loudness/<token>` 側で token 有効性を先に確認した後にのみ JSON を返す（404 / 204 と通常応答で挙動を切り分け）。
  - CORS は local app 用に `*` 許可で固定。本実装は audio server に既存の `/audio` でも CORS を付けず動作していた経緯から、loudness JSON のみ CORS を付与した。
- Deferred 残件:
  - 50 ms 窓・smoothing 係数の体感調整は実機での主観評価を経て微調整する余地あり（`Visualizer.tsx` の `0.55` / `0.35`）。
  - WAV envelope のオフライン書き出しキャッシュ（再起動跨ぎ）は今回スコープ外。
  - 手動再生検証（`mise x -- wails dev` で BGM / Talk / silence / skip / volume slider）は実機側で実施が必要。

## Follow-up Review Implementation 2026-06-13

ユーザーから提示されたレビュー指摘への対応。本対応は挙動の変更を伴わず、テスト追加とフロントエンドの読みやすさ改善に限定する。

- `internal/audio/server_test.go` を新規追加。`audiofmt` 既存テストと整合する。
  - テストヘルパー `writeSineWav` / `writeRawFile` / `newTestServer` / `doGet` を導入し、WAV 入力は `audiofmt.EncodeWavPCM16` で組み立てる。
  - `TestServerRegisterFile_ReturnsLoudnessURLForWAV`:
    - 16-bit PCM WAV を `RegisterFile` し、`LoudnessURLForAudioURL` が `BaseURL() + "/loudness/" + token` 形になることを確認。
    - `GET /loudness/<token>` が `200` JSON、Content-Type `application/json`、`Access-Control-Allow-Origin: *` を返すことを確認。
    - レスポンスを `LoudnessEnvelopeResponse` にデコードし、`WindowMS=50`、RMS 非空、Peak が `(0, 1]` であることを検証。
  - `TestServerRegisterFile_NonWAVReturns204`:
    - 拡張子を持たない raw バイトを `RegisterFile` し、audio URL が返ること、`/loudness/<token>` が `204` で空ボディを返すことを確認。
  - `TestServerLoudness_UnknownTokenReturns404`:
    - 未知 token の `GET /loudness/<token>` が `404` を返すことを確認。
  - `TestServerLoudness_ExpiredTokenReturns404AndDropsCache`:
    - `RegisterFile(path, time.Nanosecond)` + 短い sleep で失効させ、`/loudness/<token>` が `404` を返すことを確認。
    - 内部状態 `s.mu` を package プライベートフィールド（`s.tokens`, `s.loudness`）から直接参照し、失効時に両 cache が消えていることを検証。
  - `TestServerLoudness_OPTIONSPreflightReturns204WithCORS`:
    - `OPTIONS /loudness/<token>` が `204` を返し、`Access-Control-Allow-Origin: *` と `Access-Control-Allow-Methods` を含むことを確認。
- サーバー側実装（`internal/audio/server.go`）には変更なし。token 失効時の cache 削除（`handleAudio` / `handleLoudness` の両方で `delete(s.loudness, tok)` を実行）が既存コードでカバー済みであることをテストで確認。
- `frontend/src/App.tsx` の loudness fetch effect を読みやすく整理。挙動は同等。
  - 旧: `try { fetch + !res.ok + res.json() }` を 1 つの `try/catch` で囲み、`!res.ok` 以外の 2xx/4xx/5xx/空ボディの parse 失敗を暗黙の `catch` に依存していた。
  - 新: `fetch` のネットワーク失敗を最初の `try/catch` で受ける。続いて `if (res.status === 204 || !res.ok) return;` でステータスチェックを明示。`res.json()` 呼び出しを独立した `try/catch` で囲み、空ボディや壊れた JSON を「parse 失敗 → silent fallback」として明示。
  - 既存の silent fallback、`cancelled` チェック、`currentIdRef` による stale fetch 破棄、clamp 動作はすべて維持。
- 検証:
  - `mise x -- go test -v -count=1 ./internal/audio ./internal/audiofmt` 全パス（5 + 7 = 12 件）。
  - `mise x -- go test -count=1 ./...` 全パス（audio / audiofmt / musicgen / store）。
  - `mise x -- npm run build` 成功（`tsc && vite build`、`dist/assets/index.b67d83ec.js` 等を生成）。
- 非対応 / メモ:
  - レビュー指摘にあった「Web Audio / FFT / polling」は今回スコープ外（設計上の Non-Goals）。
  - テスト WAV は 1 kHz / 1 ch / 200 ms / 振幅 16384 で 50 ms 窓が 4 個生成されるものを共通基盤として使用。`sine` の絶対ピークは振幅 16384 に近いが、窓内の位相平均のため RMS はむしろ小さくなる。`Peak` の `(0, 1]` 検証は絶対最大値を見るため安定。
  - `LoudnessURLForAudioURL` の戻り値を `BaseURL() + "/loudness/" + filepath.Base(url)` で検証したが、これは `BaseURL` を介した乱暴な check。`baseURL + "/audio/" + tok` → `baseURL + "/loudness/" + tok` への置換規則が分かっていれば十分。
