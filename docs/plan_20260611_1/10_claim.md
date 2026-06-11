# Claim

## Purpose

`fm-live-radio` を、手元の音楽ファイルと Gemini TTS に依存するラジオから、ローカル生成の音楽とローカル生成の音声で継続再生できる AI ラジオへ作り替える。

## Background

現行アプリは Wails v2 + Go + React で、BGM フォルダ内の曲をシャッフル再生し、RSS 記事を LLM でトーク原稿化して Gemini TTS の音声を曲間に挟む。これにより次の制約がある。

- 音楽は手元ファイルの有限集合をループする。
- トーク音声は Gemini TTS API 料金とネットワークに依存する。
- 真に無限生成するには、BGM と Talk の両方をローカル生成へ移行する必要がある。

ユーザーは次の調査済み Go 移植リポジトリを指定している。

- Stable Audio 3 small-music Go 移植調査: `E:\programming\AI_generative\VibeCoding\stuble-audio-3-research`
- IrodoriTTS Go 移植調査: `E:\programming\AI_generative\VibeCoding\tts-research`

## Problem

現行の BGM 選曲と Gemini TTS は、ラジオ体験を継続させるための素材供給源として有限または有料である。これを解消するには、アプリ内バックエンドで次を実現する必要がある。

- BGM スロットで Stable Audio 3 による新規音楽 WAV を生成する。
- Talk スロットで IrodoriTTS による日本語読み上げ WAV を生成する。
- 生成時間が再生体験を止めないよう、先読み、キャッシュ、フォールバックを設計する。
- 巨大 ONNX モデルと ONNX Runtime DLL を Wails アプリで扱う配置、設定、初期化、検証方法を決める。

## Target Users / Environment

- 対象ユーザー: ローカル PC で自分用の AI ラジオを連続再生したいユーザー。
- 対象 OS: まず Windows x64。現行 Wails アプリと調査済みリポジトリが Windows + Go + ONNX Runtime を前提にしているため。
- 実行前提:
  - `mise` 管理下の Go / Node / Wails。
  - CGO が有効な Go ビルド。
  - MSYS2 UCRT64 `gcc.exe`。
  - ONNX Runtime 1.26.0 の共有ライブラリ。
  - Stable Audio 3 / IrodoriTTS のモデルファイルは巨大なため、リポジトリに直接コミットしない。

## Initial Scope

MVP は次を対象にする。

- 現行の RSS + OpenAI 互換 LLM による原稿生成は維持する。
- Gemini TTS を IrodoriTTS ローカル音声合成に置き換える。
- 手元 BGM フォルダ選曲を Stable Audio 3 ローカル音楽生成に置き換える。
- 既存の `BGM x N -> Talk x 1` サイクルと silence gap は維持する。
- 生成済み WAV を現行のローカル HTTP audio server で配信する。
- 設定画面からローカル生成モデルのパス、生成秒数、seed、プロンプト系設定を保存できるようにする。

## Initial Technical Hypotheses

- 既存 `internal/talk.Service` は `RSS -> LLM -> TTS -> temp_audio WAV` の境界が明確であり、TTS クライアント差し替えに適している。
- 既存 `internal/bgm` はファイル列挙と選曲に特化しているため、ローカル生成 BGM は新規 `internal/musicgen` などに分離し、`player` が `bgm` または `musicgen` を選べる構成にするのが安全。
- Stable Audio 3 / IrodoriTTS の調査リポジトリはどちらも `internal` パッケージ中心なので、そのまま Go module import するより、統合用に必要パッケージを vendoring またはサブモジュール化する方が現実的。
- ONNX Runtime はプロセスグローバル初期化が必要なため、SA3 と IrodoriTTS で別々に `ort.Init` しない。`fm-live-radio` 側に共通 ORT ランタイム管理層を置く。

## Confirmed Decisions

- ユーザー向け表記は `Stable Audio 3` とする。`Stuble Audio 3` は typo。
- IrodoriTTS は v3 を使用する。
- IrodoriTTS v3 は `narrator` ディレクトリ内の声質 WAV を参照する。声質 WAV がない場合はデフォルト話者にフォールバックする。
- モデルは `model` ディレクトリ、声質 WAV は `narrator` ディレクトリ、生成音楽キャッシュは `generate_music` ディレクトリに置く。
- これらの基準ディレクトリは exe ファイルのパス、または開発時のプロジェクトルートとする。
- `generate_music` は約 20 個の生成音楽キャッシュを保持し、それ以上は古いものから削除する。
- Stable Audio 3 の生成が間に合わない場合は、`generate_music` キャッシュから選んで再生する。
- `narrator` に複数 WAV がある場合は、ファイル一覧取得時の 1 番目を参照声質 WAV として使う。意図的な声選択ではなく、ファイルシステム都合の順序でよい。
- `generate_music` のフォールバック再生では、古い順に並べたキャッシュの `n/2` 番目付近を選ぶ。最古ファイルは再生中にキャッシュ削除対象になりやすいため避ける。

## Uncertainties

- 現行 Wails アプリの Go 版は `go.mod` で `go 1.23`、調査リポジトリは Go 1.25 系を前提にしている。統合時に Go バージョンを上げる必要がある可能性が高い。
- 生成速度が曲長に対して十分かは、統合先での実測が必要。再生が追いつかない場合は短い曲、長い先読みキュー、フォールバック音源が必要。
- モデルアセットの配布方法、初回セットアップ、ライセンス表示は未確定。

## Next Documents

- `20_app_requirement.md`: ユーザー体験と MVP 範囲。
- `docs/cheatsheet/local-generation-research.md`: 調査リポジトリから再利用すべき技術知識。
- `30_requirement.md`: 機能・非機能・制約・受け入れ条件。
- `40_specification.md`: 実装境界、型、ファイル構成、検証設計。
- `50_review_notes.md` 以降: 承認前レビュー用のリスク、判断、チェックリスト。
