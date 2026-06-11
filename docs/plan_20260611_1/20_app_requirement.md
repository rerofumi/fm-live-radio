# App Requirements

## Purpose

ローカル生成の音楽とローカル生成の音声を組み合わせ、API 料金と固定音源ループに依存しない AI ラジオ体験を提供する。

## Target Environment

- Windows x64 desktop。
- Wails v2 アプリ。
- Go / Node / Wails は `mise` 経由で実行する。
- `.jj` が存在するため、リポジトリ操作は jj を優先する。
- モデル推論は ONNX Runtime 1.26.0 + CPU EP を初期ターゲットにする。
- Windows の CGO ビルドには MSYS2 UCRT64 gcc を必要とする。
- モデルファイルは基準ディレクトリ配下の `model` に配置する。基準ディレクトリは packaged exe のカレントディレクトリ、または開発時のプロジェクトルートとする。
- 声質 WAV は基準ディレクトリ配下の `narrator` に配置する。
- Stable Audio 3 生成音楽は基準ディレクトリ配下の `generate_music` に保存する。

## User Experience

- ユーザーは再生ボタンを押すだけで、生成音楽とニューストークが交互に流れる。
- 既存のラジオ UI は大きく壊さず、現在の BGM / Talk 表示、再生・停止・スキップ、音量、生成中ランプを維持する。
- Settings で次を設定できる。
  - BGM ソース: `local files` / `Stable Audio 3`
  - Talk TTS: `Gemini` / `IrodoriTTS v3`
  - Stable Audio 3 モデルディレクトリ、生成秒数、steps、seed 方針、基本プロンプト、ムード指定
  - IrodoriTTS v3 モデルディレクトリ、参照声質 WAV、steps、秒数・自動長設定
  - RSS URL、LLM 接続、Talk cycle、音量、silence gap
- 生成が間に合わない場合でもアプリは停止せず、状態表示とフォールバックで継続する。

## MVP Features

- **ローカル音楽生成**
  - Stable Audio 3 small-music で指定秒数のステレオ WAV を生成する。
  - 生成プロンプトは設定の基本プロンプトと、簡易ムード・時間帯・番組名から構成する。
  - 生成済み WAV は基準ディレクトリ配下の `generate_music` に保存し、ローカル HTTP audio server で配信する。
  - `generate_music` は約 20 個のキャッシュとして扱い、上限超過時は古いファイルから削除する。
  - Stable Audio 3 生成が間に合わない場合は、`generate_music` の既存キャッシュから選んで再生する。
  - 直近生成プロンプト・seed は PlayableItem の source に保持し、デバッグできるようにする。

- **ローカル Talk TTS**
  - RSS + OpenAI 互換 LLM の原稿生成は維持する。
  - TTS 実行を Gemini REST から IrodoriTTS v3 pipeline へ差し替え可能にする。
  - 基準ディレクトリ配下の `narrator` にある声質 WAV を参照音声として使う。
  - 声質 WAV がない場合は IrodoriTTS v3 のデフォルト話者で生成する。
  - 生成 WAV は既存 Talk 音声と同じ再生経路に乗せる。

- **先読み**
  - BGM 生成と Talk 生成はバックグラウンドで先読みする。
  - 同時に重い ONNX 推論を複数走らせないため、生成ワーカーは最初は 1 本に制限する。
  - 再生中の曲が終わる前に次の Stable Audio 3 音楽ができていない場合は、`generate_music` キャッシュからフォールバック再生する。

- **互換モード**
  - 既存のローカル BGM フォルダ再生と Gemini TTS は即削除しない。
  - ローカル生成が未設定または失敗したときのフォールバックとして利用できる。

## Future Candidates

- ニュース本文生成もローカル LLM に固定し、完全オフライン化する。
- Stable Audio 3 small-sfx によるジングル生成を追加する。
- 生成曲の軽量キャッシュと番組ごとのプレイリスト風履歴を追加する。
- IrodoriTTS v3 の narrator WAV 明示選択 UI を追加する。
- DirectML / CUDA / TensorRT など CPU 以外の EP 対応を検討する。
- 番組ペルソナ、声、音楽プロンプトをプリセット化する。

## Initial Non-Goals

- Stable Audio 3 Medium / Large の対応。
- モデルファイルのアプリ同梱配布。
- 生成音楽の厳密なビートマッチ、クロスフェード、ラウドネス正規化の完全自動化。
- IrodoriTTS の学習、LoRA、声質作成。
- 完全な無音ゼロの放送品質ギャップレス再生。
- Web 配信サーバー化。
