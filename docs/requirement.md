# Current Requirements

最終確認日: 2026-06-13

この文書は、現在実装されている `fm-live-radio` の要求仕様を示す。`docs/plan_*` の検討内容ではなく、現行コードと一致する要件のみを記載する。## 目的

`fm-live-radio` は、ローカルデスクトップ上で BGM と AI 生成ニューストークを交互に再生する「AI ローカルラジオ」アプリである。ユーザーはニュース RSS、LLM、ローカル推論の設定を行い、連続再生されるラジオ風の体験を得られる必要がある。

## 対象環境

- Windows x64 を主な対象環境とする。
- Wails + Go + React によるデスクトップアプリとして動作する。
- 開発・検証コマンドは `mise` 経由で実行できる必要がある。
- OpenAI 互換 Chat Completions API を利用できる環境を前提とする。
- Stable Audio 3 および IrodoriTTS v3 を使うため、対応するローカルモデルと ONNX Runtime が利用可能である必要がある。

## 機能要件

### 再生体験

- アプリは Play / Pause / Skip による基本的な再生操作を提供する。
- 再生対象は BGM、Talk、無音ギャップを扱える必要がある。
- BGM と Talk の間には、設定された範囲内の無音ギャップを挿入する。
- BGM を一定曲数再生した後に Talk を差し込む。
- BGM 音量と Talk 音量は個別に調整できる必要がある。
- 現在再生中の種別、タイトル、進捗、再生時間を UI に表示する。
- Talk と Music の生成・先読み状態を UI に表示する。
- ローカル生成の警告またはエラーは UI に表示できる必要がある。
- メイン UI はラジオ筐体風の外観を持ち、Settings は右上から常時開ける必要がある。
- BGM と Talk の音量は個別のツマミ風コントロールとして表示し、変更時に即時保存できる必要がある。
- Stable Audio 3 のジャンルはチューニングメーター風 UI から固定 4 値を選択でき、現在再生中または先読み中の BGM を中断せず次回生成へ反映できる必要がある。
- 画面幅が狭い場合、操作パネルとチューニングメーターは縦積みになり、横スクロールを前提にしない。
- 再生中の BGM / Talk について、ファイル先頭からの音量包絡（loudness envelope）を視覚化に反映する。Web Audio や FFT は使わない。
- 包絡が取得できない場合・無音 gap・一時停止時は、既存の合成アニメーションへ静かに fallback する。
- 現在の音量 slider の値を視覚化の振幅にも乗算する。
- item 切替・skip 時に、前 item の包絡が次 item の描画に残らない。
- reduced-motion 設定時は連続アニメーションを再開しない。

### BGM

- BGM は Stable Audio 3 モデルを使って BGM WAV を生成する。
- モデルディレクトリ、出力ディレクトリ、prompt base、ジャンル、秒数、steps、seed mode、fixed seed、cache limit を設定できる。
- ジャンルは固定の 4 値から選択する。`chill lo-fi`, `smooth jazz`, `minimal electronica`, `ambient music`。
- 既定ジャンルは `chill lo-fi`。`config.json` の `stableAudio3.genre` が空または未対応値の場合は既定値へ正規化される。
- Stable Audio 3 に渡す prompt では、選択ジャンル名だけでなく、楽器、音色、リズム、雰囲気を含むジャンル説明文へ展開される必要がある。
- Stable Audio 3 の生成に失敗した場合、利用可能な生成済み WAV があれば fallback として使える必要がある。
- 生成済み BGM cache は設定された上限に基づいて整理される必要がある。

### Talk

- Talk は RSS 記事選択、LLM 原稿生成、IrodoriTTS による音声合成の順で生成される。
- RSS URL は複数設定できる。
- 過去に利用した記事 URL は履歴に保存し、再利用を避ける。
- RSS item の本文が不足する場合、記事ページから本文抽出を試みる。
- LLM は OpenAI 互換 `/chat/completions` API を利用する。
- LLM base URL、API key、model を設定できる。
- Talk 原稿はラジオ DJ 風の短いニュース紹介として生成される。
- Talk 生成結果は一時 WAV ファイルとして保存され、再生できる必要がある。

### IrodoriTTS v3

- IrodoriTTS v3 のモデルディレクトリを設定できる必要がある。
- narrator ディレクトリと任意の参照 WAV path を設定できる必要がある。
- 参照 WAV path が空の場合、narrator ディレクトリ内の WAV を参照音声として利用できる必要がある。
- 参照 WAV が見つからない場合でも、参照音声なしで合成を試みる。
- 長い Talk 原稿は文単位に分割して合成される必要がある。
- 文単位の合成に失敗した場合、全体を即座に失敗させず、無音で置き換えて合成を継続する。
- Irodori の steps、seed mode、fixed seed、CFG 値、duration scale を設定できる必要がある。

### ローカル推論

- Stable Audio 3 と IrodoriTTS v3 は共有の ONNX Runtime 初期化機構を使う。
- ONNX Runtime DLL path は設定値または環境変数から指定できる。
- execution provider は `auto`、`cuda`、`cpu` を選択できる。
- `auto` は CUDA が利用できない場合 CPU に fallback する。
- `cuda` は CUDA を強制し、利用できない場合はエラーにする。
- device ID を設定できる必要がある。
- 初期化済みの ORT DLL path または execution provider を変更する場合は、アプリ再起動を必要とする。

### 設定と履歴

- 設定は OS の user config directory 配下に `config.json` として保存される。
- 記事利用履歴は `history.json` として保存される。
- 履歴の article URL は最大 500 件に制限される。
- Talk などの一時音声は user config directory 配下の `temp_audio/` に保存される。
- 起動時に `temp_audio/` の古いファイルは best-effort で削除される。

## 非機能要件

- ローカル audio server は `127.0.0.1` の動的 port で起動し、外部公開を前提にしない。
- 再生用 audio URL は token 付きで発行し、一定時間後に無効化される。
- API key はログに積極的に出力しない。
- 生成処理は UI 操作を長時間ブロックしないよう、Talk と Music の prefetch を利用する。
- ローカル生成による BGM / Talk の基本再生フローが維持される。
- loudness envelope の取得失敗や、非 WAV ファイル / 旧 token に対して 204 / 404 が返っても、`RegisterFile` による audio URL 発行と再生は失敗させない。
- loudness envelope の参照は描画 frame ごとに network polling を行わない。item 切替時に一度だけ取得する。

## 制約

- ローカル生成モデル、GPU 版 ONNX Runtime、CUDA/cuDNN 関連 DLL はリポジトリ管理対象外である。
- Talk は RSS URL が空の場合、生成できない。
- IrodoriTTS は必要な model asset が不足している場合、生成できない。
- RSS 記事本文抽出はサイト構造に依存するため、十分な本文を取得できない場合がある。
- ORT はプロセス内で一度初期化されるため、実行中の provider 切り替えには対応しない。

## 対象外

- RSS 以外のニュースソース連携。
- 複数プレイリストや予約番組表の管理。
- クラウド同期。
- 設定ファイルの暗号化。
- ローカル生成モデルの自動取得。ただし GPU 用 ORT 取得補助スクリプトは存在する。
- 本格的な記事重複判定。現状は article URL 履歴に基づく。

## 受け入れ条件

- `mise x -- go test ./...` が実行可能である。
- `mise x -- npm --prefix frontend run build` が実行可能である。
- `mise run build` が実行可能である。
- Settings から RSS、LLM、Stable Audio 3、Irodori、ORT provider 関連設定を保存できる。
- Wails 起動時の標準ウィンドウは横長のラジオ UI を表示し、最小ウィンドウサイズにより極端な縮小を抑止できる。
- `stable_audio_3` 設定済みモデルと ORT があれば BGM を生成して再生できる。
- `irodori` 設定済みモデルと ORT があれば Talk WAV を生成して再生できる。
- `auto` provider で CUDA が利用できない場合、CPU fallback の警告が UI status に反映される。
