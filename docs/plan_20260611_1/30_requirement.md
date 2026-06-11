# Requirements

## Scope

本計画は `fm-live-radio` のバックエンド生成基盤と設定 UI を拡張し、BGM と Talk 音声をローカル生成に差し替え可能にする。現行の RSS、LLM 原稿生成、Wails フロントエンド、ローカル audio server は可能な限り維持する。

## Functional Requirements

### FR-1: 生成ソース選択

- 設定に BGM ソース種別を追加する。
  - `files`: 現行の BGM フォルダ再生。
  - `stable_audio_3`: Stable Audio 3 によるローカル生成。
- 設定に TTS ソース種別を追加する。
  - `gemini`: 現行の Gemini TTS。
  - `irodori`: IrodoriTTS によるローカル生成。
- 既存 config は読み込み時にデフォルト値でマイグレーションされる。

### FR-2: Stable Audio 3 BGM 生成

- Stable Audio 3 small-music のモデルディレクトリ、生成秒数、steps、seed 方針、基本プロンプトを設定保存できる。
- BGM スロットで Stable Audio 3 を実行し、ステレオ WAV を生成する。
- 生成結果は `PlayableItem{kind:"bgm"}` として返り、既存 audio server URL で再生できる。
- 生成結果は基準ディレクトリ配下の `generate_music` に保存する。
- `generate_music` には約 20 個の生成音楽をキャッシュし、上限超過時は古いファイルから削除する。
- 生成が間に合わない場合は `generate_music` キャッシュから選んで再生する。
- フォールバック再生では、`generate_music` を古い順に並べた `n/2` 番目付近のファイルを選ぶ。最古ファイルはキャッシュ削除と競合しやすいため避ける。
- キャッシュが空で生成も間に合わない場合は、設定不備または初回生成待ちとして UI に表示する。

### FR-3: IrodoriTTS v3 Talk 音声生成

- RSS 選択と LLM 原稿生成は現行 `talk.Service` の流れを維持する。
- OpenAI 互換 LLM の台本生成は、thinking 対応モデルが推論トークンを消費しても本文を返せるよう、出力上限を十分に確保する。
- LLM が空白のみの台本を返した場合、TTS に渡さず Talk 生成失敗として扱う。
- Gemini TTS の代替として IrodoriTTS v3 pipeline を呼び出し、WAV を生成する。
- Gemini TTS は長文一括生成の品質が高いため、台本全体を一括で TTS に渡す現行挙動を維持する。
- IrodoriTTS v3 provider は長い台本を短いセンテンスに分割し、センテンスごとに音声生成してから 1 つの WAV に結合する。
- IrodoriTTS v3 provider で個別センテンス生成に失敗した場合、MVP ではそのセンテンス位置に 3 秒の無音を挿入して Talk 全体の生成を継続する。
- IrodoriTTS v3 モデルディレクトリ、speaker mode 用の参照 WAV、steps、seconds、duration scale を設定保存できる。
- 参照 WAV は基準ディレクトリ配下の `narrator` から選ぶ。
- `narrator` に複数 WAV がある場合は、ファイル一覧取得時の 1 番目を使う。
- `narrator` に声質 WAV が存在しない場合は、IrodoriTTS v3 のデフォルト話者で生成する。
- TTS 失敗時は Talk をスキップし、BGM サイクルへ復帰する。

### FR-4: ONNX Runtime 管理

- Stable Audio 3 と IrodoriTTS で共通の ONNX Runtime 初期化層を持つ。
- ORT DLL パスは設定または環境変数で上書きできる。
- ORT 初期化失敗、モデル不足、CGO 環境不備は UI に表示できるエラーとして返す。

### FR-5: 生成キューと先読み

- BGM 生成、Talk 生成はバックグラウンドで先読みできる。
- 最初の MVP では同時推論数を 1 に制限する。
- 先読み状態は `AppStatus` に反映する。
- 設定変更、停止、スキップ時に不要な生成ジョブをキャンセルまたは破棄できる。

### FR-6: ファイル保存とクリーンアップ

- Talk 音声などの一時生成物はユーザー設定ディレクトリ配下に保存する。
- Stable Audio 3 の生成音楽は基準ディレクトリ配下の `generate_music` に保存する。
- `generate_music` は約 20 個のキャッシュとして保持し、古いファイルから削除する。
- Talk 音声などの一時生成物は起動時または容量上限に基づいて掃除する。
- モデルファイル、ONNX Runtime DLL、巨大な生成キャッシュは git 管理対象外にする。

### FR-7: UI 設定

- Settings にローカル生成用の設定項目を追加する。
- モデルディレクトリは基準ディレクトリ配下の `model` を既定値にする。
- 声質 WAV ディレクトリは基準ディレクトリ配下の `narrator` を既定値にする。
- 生成音楽ディレクトリは基準ディレクトリ配下の `generate_music` を既定値にする。
- モデルディレクトリ、narrator ディレクトリ、generate_music ディレクトリ、ORT DLL パスは必要に応じて手入力またはフォルダ選択で上書きできる。
- 生成中、準備完了、フォールバック中、設定不備をユーザーに表示する。

## Non-Functional Requirements

- 生成失敗でアプリ全体をクラッシュさせない。
- API キーやローカルファイルパスはログに不用意に出さない。
- 重い ONNX モデルロードは必要時に 1 回だけ行い、セッションを再利用する。
- 再生 UI は生成待ち中でも操作不能にしない。
- 設定変更後の再初期化は明示的に成功・失敗が分かる。
- Windows の長いパス、空白を含むパスを扱える。

## Constraints

- 実装前に本計画の承認が必要。
- `mise.toml` に task があるため、ビルド・テスト・Wails 操作は `mise` 経由で実行する。
- `.jj` があるため、状態確認や履歴操作は jj を優先する。
- Go / npm / wails は原則直接実行しない。
- Python や pip が必要な場合は uv を介する。
- 現行 `docs/` 直下には最新仕様がないため、実装完了後に current docs を新設または更新する必要がある。

## Compatibility

- 既存 config を壊さない。未知フィールドがない古い `config.json` はデフォルトで補完する。
- 既存ローカル BGM フォルダ再生と Gemini TTS は移行用の互換モードとして残す。
- Wails バインディング変更後は `mise x -- wails generate module` を実行する。
- `frontend/wailsjs` の生成物は Go API 変更に追従させる。

## Out of Scope

- Stable Audio 3 Medium / Large。
- モデルダウンローダー UI。
- モデルファイルの再配布・ライセンス同梱の最終整理。
- ローカル LLM 自体の同梱。
- 生成音楽の著作権・商用利用ポリシー判断。

## Acceptance Criteria

- AC-1: `files + gemini` の既存相当モードで従来再生が動作する。
- AC-2: `stable_audio_3 + gemini` で BGM が Stable Audio 3 生成 WAV に置き換わる。
- AC-3: `files + irodori` で Talk 音声が IrodoriTTS v3 生成 WAV に置き換わる。
- AC-4: `stable_audio_3 + irodori` で、ローカル生成 BGM と IrodoriTTS v3 Talk が `BGM x N -> Talk` サイクルで再生される。
- AC-5: モデル未設定、ORT DLL 不足、生成失敗時にアプリが停止せず、UI に原因が表示される。
- AC-6: 生成音声が既存 audio server 経由で再生できる。
- AC-7: `mise x -- go test ./...` が通る。
- AC-8: Go API 変更後に `mise x -- wails generate module` が成功する。
- AC-9: 少なくとも 1 回、短い Stable Audio 3 生成と IrodoriTTS 生成の smoke test を統合先で実行し、WAV が無音でないことを確認する。
- AC-10: `generate_music` が 20 個程度を超えたとき、古い生成音楽が削除される。
- AC-11: Stable Audio 3 生成が間に合わないとき、`generate_music` にキャッシュがあればそこから BGM が再生される。
- AC-12: `narrator` に声質 WAV がない場合でも IrodoriTTS v3 のデフォルト話者で Talk が生成される。
- AC-13: `narrator` に複数 WAV がある場合、ファイル一覧取得時の 1 番目が参照 WAV として使われる。
- AC-14: `generate_music` フォールバックでは、古い順の `n/2` 番目付近が選ばれ、最古ファイル再生中の削除競合を避ける。
- AC-15: `gemma4:12b` のような thinking 対応 OpenAI 互換モデルで、LLM 台本生成の `max_tokens` 不足により空台本が TTS に渡らない。
- AC-16: `ttsSource=irodori` では長いニュース台本がセンテンス単位に分割生成され、成功 WAV と失敗箇所の 3 秒無音が結合された 1 つの Talk WAV として再生される。
- AC-17: `ttsSource=gemini` ではセンテンス分割せず、従来どおり台本全体を一括 TTS 生成する。
