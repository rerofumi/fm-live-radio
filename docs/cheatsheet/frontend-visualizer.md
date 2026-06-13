# Frontend Visualizer (波形ビジュアライザ)

最終確認日: 2026-06-13

`fm-live-radio` の「無限に音が流れ続ける」コンセプトを表現する、常時アニメーションする波形ビジュアライザ (`frontend/src/Visualizer.tsx`) の実装知見。

## 結論

- ビジュアライザは **実音声の FFT 解析を使わず**、再生状態 (`playing` / `kind` / `level`) で駆動する **合成アニメーション** で描画する。
- Canvas 2D + `requestAnimationFrame`。複数の正弦波レイヤーの位相を毎フレーム進め、横方向に流れ続ける波形を描く。
- 目標値 (振幅 / エネルギー / 速度 / 色相) を状態から決め、現在値をフレームレート非依存に補間 (`k = 1 - pow(c, dt)`) してなめらかに遷移させる。
- アイドル/一時停止でも振幅・速度を 0 にせず、常に静かに流れ続ける（コンセプト表現）。

## なぜ実 FFT を使わないか

- BGM/Talk の音声はローカル audio server (`127.0.0.1:<dynamic port>`) から `<audio>` の `src` に渡される。これはアプリ本体とは **別オリジン**。
- Web Audio の `createMediaElementSource()` でこの要素を解析グラフに接続しても、CORS ヘッダのないクロスオリジン音源は **解析データが無音 (0) になり**、波形が動かない。
- さらに `MediaElementSource` に接続すると音声経路が Web Audio グラフ経由になり、`AudioContext` の suspend 等で **再生自体を壊すリスク** がある。
- audio server に CORS を付与し token URL を解析可能にする改修も可能だが、コンセプト上「常時動く」ことが重要で、合成アニメーションの方が確実かつ安全。

## 状態と見た目の対応

- idle / paused: 低振幅・低速・インディゴ。止まらず静かに流れる。
- bgm: 高振幅・高エネルギー・インディゴ〜バイオレット。
- talk: 中振幅・暖色 (アンバー) で BGM と区別。
- silence (間): 最小振幅・寒色。

## アクセシビリティ / 性能

- DPR は 1〜2 に clamp し、`ResizeObserver` で実サイズに追従。
- `prefers-reduced-motion: reduce` 時は rAF ループを止め、状態変化時のみ静止フレームを再描画する。
- 描画用の `<canvas>` ラッパには `aria-hidden="true"` を付与（情報は title/now playing 側で提供）。
