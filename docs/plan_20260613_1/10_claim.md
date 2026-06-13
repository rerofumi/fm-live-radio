# Claim

## Purpose

ローカル生成が実用段階に入ったため、過去の互換要素であるファイル BGM と Gemini TTS を削除し、設定画面と config をローカル生成中心に整理する。

## Background

現行実装は、移行期間の互換性として次の旧 provider を残している。

- BGM source: `files`
- TTS source: `gemini`

現在の主経路は Stable Audio 3 による BGM 生成と IrodoriTTS v3 による Talk 音声生成であり、旧 provider が残ることで設定画面、config、実行分岐、ドキュメントが複雑になっている。

## Problem

- Settings に現在使わない `BGM Source`、`TTS Source`、`BGM Root Path`、`Gemini API Key`、Gemini TTS model / voice が残っている。
- `AppConfig` に旧 provider 用 field が残っており、保存 config が目的に対して冗長である。
- `player` と `talk` に旧 provider 分岐が残り、ローカル生成だけを前提にした保守がしにくい。
- Settings は機能設定と生成設定が混在しており、日常的に触る項目と詳細な ONNX / model 設定の見通しが悪い。

## Target Users / Environment

- Windows x64 の Wails デスクトップアプリ利用者。
- ローカルモデルと ONNX Runtime を設定済み、または設定するユーザー。
- 開発検証は `mise` 経由の Go / frontend build を使う。

## Initial Scope

- `AppConfig` から旧 provider の選択・設定 field を削除する。
- BGM は Stable Audio 3 生成のみ、Talk TTS は IrodoriTTS v3 のみとする。
- Settings を「アプリ機能」と「生成設定」に分ける。
- 「生成設定」はデフォルトで閉じたアコーディオンにする。
- `ScanGenres` と genre UI はファイル BGM 依存なので削除する。
- 旧 Gemini TTS 実装ファイルは参照が消えるなら削除する。
- 現行 docs / README を実装後の事実に合わせて更新する。

## Initial Technical Hypotheses

- Go の `encoding/json` は struct から削除された旧 field を読み捨てるため、既存 `config.json` に `bgmRootPath` や `geminiApiKey` が残っていても load は失敗しない。
- `BGMSource` / `TTSSource` enum を削除しても、生成済み Wails TS binding を更新すれば frontend build は通る。
- `internal/bgm` と `internal/tts/gemini_tts.go` は参照が消えた後に削除できる。
- `PlayableSource.FilePath` は生成 BGM の出力 path 表示にも使われているため、旧ファイル BGM削除後も残す。

## Uncertainties

- Wails generated files は `mise x -- wails generate module` で更新できる想定だが、ローカル Wails CLI が未導入の場合は `mise run setup` が必要。
- Settings のアコーディオンは標準 `details/summary` で足りる想定。既存デザインに合わせた CSS 調整は実装時に確認する。

## Next Documents

1. `20_app_requirement.md`
2. `30_requirement.md`
3. `40_specification.md`
4. `50_review_notes.md`
5. `60_review_brief.md`
6. `70_review_board.html`
7. `80_review_checklist.md`
