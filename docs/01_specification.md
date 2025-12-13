# 01_specification.md — 実装仕様書（要求仕様書→実装ブリッジ）

- プロジェクト: **AI ローカルラジオアプリ（仮称）**
- 作成日: 2025-12-13
- 対象: `docs/00_requiments.md` を実装に落とし込むための具体仕様（画面・データ・API・状態遷移・例外処理）

---

## 0. 本書の目的 / 前提

### 0.1 目的
- 要求定義（`00_requiments.md`）を、**Wails(Go)+React**で実装可能な粒度に分解する。
- フロントエンド（再生制御）とバックエンド（BGMスキャン/設定/AI生成）の境界とI/F（インターフェース）を確定する。

### 0.2 対象外（本版でやらない）
- クロスフェード / 厳密なギャップレス再生
- 複数ペルソナ切替
- 天気予報連携
- BGMの自動ムード分類

### 0.3 用語
- **BGM**: ユーザー指定ルート配下のサブフォルダ（ジャンル）にある音楽ファイル
- **Talk**: RSS記事をネタにしたAI生成トーク音声（約1分）
- **サイクル**: `BGM×3 → Talk×1` の繰り返し
- **History**: Talkで利用済みの記事URL（重複排除のため）

---

## 1. 全体構成（実装方針）

### 1.1 技術スタック
- Desktop App: **Wails v2**（Go + Webview）
- Frontend: **React**（UI + HTML5 Audio 再生制御）
- Backend: **Go**（BGMスキャン、設定永続化、RSS/LLM/TTS 呼び出し）

### 1.2 コンポーネント責務

#### Frontend（React）
- 画面描画、ユーザー操作
- Audio再生（`<audio>` または `new Audio()`）
- `onended` / `onerror` を起点に次アイテム要求
- 簡易状態（再生中/停止、音量、現在表示メタ情報）

#### Backend（Go）
- BGMルートをスキャンし、ジャンル一覧と曲一覧を構築
- 次のBGM選択（ランダム）
- RSS取得・記事選定（History照合）
- LLMで台本生成 → TTSで音声化 → 一時ファイル管理
- 設定/履歴の永続化
- 失敗時のフェイルセーフ（Talk生成失敗ならBGMへ）

### 1.3 再生アーキテクチャ（推奨）
- **Frontend Driven**: 再生はフロントが主導し、バックエンドは「次に再生すべきアイテム（URL/メタ）」を返す。
- 音声ソースは **バックエンドがローカルで配信するファイルURL** を利用する（data URI は扱わない）。
  - 例: **バックエンド内HTTPサーバ**で `http://127.0.0.1:<port>/audio/<token>` を配布
  - 返すURLは、BGM/生成Talkのいずれも同一の仕組みで配信できること（フロントはURLを `audio.src` に設定するだけにする）。

> 要求書では Wails AssetServer への言及があるが、任意ローカルファイル配信は要件上必須。Wails標準の仕組みだけで難しければ、1) の専用HTTPで代替する。

---

## 2. 画面仕様（UI/UX）

### 2.1 画面一覧
1. **Player View（メイン）**
2. **Settings Modal（設定）**

### 2.2 Player View

#### 表示項目
- 現在再生中
  - 種別: `BGM` / `Talk` / `Silence(間)`
  - タイトル
    - BGM: 曲名（可能ならファイル名から推定）
    - Talk: RSS記事タイトル
  - サブ情報
    - BGM: アーティスト（取得可能なら。基本は任意/未対応でもOK）
    - Talk: RSSフィード名（任意）
  - 画像
    - 既定の「ラジオ局ロゴ」
    - ジャケットは将来対応（本版は固定でOK）

#### 操作
- ジャンル選択（ドロップダウン）
- 再生 / 一時停止
- スキップ（現在の音源を停止し、次を要求）
- 音量スライダー（0.0〜1.0）
- 設定ボタン（モーダルを開く）

#### 挙動
- 再生開始時、バックエンドに「次アイテム要求」し、取得できたURLを再生する。
- `onended` で次アイテムを要求する。
- `onerror`（再生不可）時は、同様に次を要求（最大リトライ回数を設ける）。

### 2.3 Settings Modal

#### 設定項目
- Gemini API Key（必須: Talk生成を使う場合）
- BGMルートフォルダ（必須: BGM再生を使う場合）
- RSS URLリスト
  - 追加
  - 削除
- LLM設定（OpenAI互換）
  - Base URL（例: `http://localhost:11434/v1` や OpenRouter のURL）
  - API Key（必要な場合）
  - Model

※ RSSは**初期値は空**（プリセットは提供しない）。

#### 保存
- 「保存」押下でバックエンドへ保存要求 → 成功でモーダルクローズ
- 保存後、BGMルート変更時はジャンル一覧を再読込

---

## 3. 永続データ仕様（config/history/temp）

### 3.1 保存場所
- OSのユーザーデータ領域（例: `os.UserConfigDir()` + アプリ名）
- 期待ディレクトリ:
  - `.../fm-live-radio/config.json`
  - `.../fm-live-radio/history.json`
  - `.../fm-live-radio/temp_audio/`（起動中に生成、終了時削除または次回起動時GC）

### 3.2 `config.json`（案）
```json
{
  "bgmRootPath": "E:/Music/BGM",
  "selectedGenre": "Lo-Fi",
  "rssUrls": [],
  "geminiApiKey": "***",
  "talk": {
    "enabled": true,
    "cycleBgmCount": 3,
    "targetDurationSec": 60,
    "silenceGapMinMs": 1000,
    "silenceGapMaxMs": 3000
  },
  "llm": {
    "enabled": true,
    "baseUrl": "http://localhost:11434/v1",
    "apiKey": "",
    "model": "gpt-4o-mini"
  }
}
```

- `llm` は要求書に「OpenAI互換(openrouter/ollama想定)」があるため **UIから編集可能** とする。

### 3.3 `history.json`（案）
```json
{
  "usedArticleUrls": [
    "https://news.example.com/a1",
    "https://news.example.com/a2"
  ],
  "updatedAt": "2025-12-13T00:00:00Z"
}
```

- 上限: 例）**500件**まで保持し、超過分は古いものから削除（肥大化防止）。

### 3.4 `temp_audio/`
- Talk生成音声を保存（例: `talk_20251213_123045.mp3`）
- 終了時削除を基本方針（削除失敗しても次回起動時に掃除）

---

## 4. BGM仕様（スキャン/メタ/選曲）

### 4.1 サポート拡張子（最小）
- `.mp3`, `.wav`, `.m4a`（実装可能な範囲で）

### 4.2 ジャンル検出
- `bgmRootPath` 直下の **サブディレクトリ名**をジャンルとして列挙
- 非表示/システムフォルダは除外（例: `.git`, `__MACOSX` など）

### 4.3 曲リスト構築
- **本版は直下のみ**を対象とする（再帰スキャンは将来必要になった時に再検討）。
- 曲名表示:
  - 既定: ファイル名（拡張子除去）
  - 将来: ID3タグ解析（本版は任意）

### 4.4 選曲アルゴリズム
- 選択ジャンル内からランダム
- 同一曲の連続再生を避ける（直前曲と同一なら引き直し。試行回数上限あり）

---

## 5. サイクル再生仕様（状態機械）

### 5.1 基本サイクル
- `BGM 1 → (間) → BGM 2 → (間) → BGM 3 → (間) → Talk → (間) → ...`
- 「間」は 1〜3秒の無音（ランダム）。

### 5.2 内部状態（例）
- `bgmCountSinceLastTalk: 0..cycleBgmCount`
- `nextTalkPrefetched: boolean`
- `prefetchedTalkRef: TalkAssetRef | null`

### 5.3 次アイテム決定
- `bgmCountSinceLastTalk < cycleBgmCount` → 次はBGM
- `bgmCountSinceLastTalk == cycleBgmCount` → 次はTalk
- Talk生成に失敗 or 準備できない → TalkスキップしてBGMにフォールバック（bgmCountは 0 に戻すか、Talk相当として進めるかを統一する）

**統一ルール（推奨）**
- Talkスロットで失敗した場合も「Talkを消化した」とみなし、`bgmCountSinceLastTalk = 0` に戻してBGMへ。

### 5.4 先読み（プリフェッチ）
- BGM再生中に次のTalkを作っておく（Low Latency目的）
- トリガ:
  - `bgmCountSinceLastTalk == cycleBgmCount-1`（= 次の次がTalk）になった時点で開始
  - もしくは `bgmCountSinceLastTalk == cycleBgmCount` の直前（BGM3開始時）
- キャンセル:
  - ジャンル変更 / 停止 / RSS設定変更 / APIキー変更 → 進行中生成をキャンセル（可能な範囲で）

---

## 6. RSS仕様（取得/選定/重複排除）

### 6.1 RSS URLリスト
- 設定で保持する複数URL
- 0件の場合: Talk生成は無効扱い（自動的にBGMのみ）

### 6.2 記事選定
1. RSS URLをランダムに1つ選ぶ
2. フィードを取得し、最新順にアイテムを走査
3. `history.usedArticleUrls` に含まれない最初のアイテムを採用
4. 未使用がない場合:
   - そのRSSは不採用として別RSSを試す（最大N回）
   - すべて失敗 → Talkスキップ

### 6.3 保存するID
- 原則: **記事URL** を保存
- URLが空/不安定な場合は GUID/ID を併用（実装で可能なら）

### 6.4 取得失敗
- ネットワークエラー、パース失敗はサイレントフェイル
- TalkスロットはBGMにフォールバック

---

## 7. 台本生成（LLM）仕様

### 7.1 入力
- 記事タイトル
- 記事本文（可能ならdescription/summary + content）
- フィード名（任意）

### 7.2 出力
- 読み上げ用テキスト（日本語）
- 目安: 200〜300文字（約1分相当）

### 7.3 システムプロンプト（例）
- 固定ペルソナ（当面固定）
- 制約:
  - 誇張しすぎない
  - 出典URLを読み上げない
  - 個人情報を生成しない

**プロンプト雛形（仕様）**
- System:
  - 「あなたは落ち着いたラジオDJ。ニュースを分かりやすく1分で紹介。口語。導入→要点→締め」
- User:
  - 「以下の記事を要約してラジオトーク原稿を作ってください。文字数は200〜300。固有名詞は必要最小限。記事: ...」

※ 実際のプロンプト文は実装で調整しやすいよう `prompt_templates` としてコード側に分離する。

### 7.4 失敗時
- LLMが失敗/タイムアウト → Talkスキップ（BGMへ）

---

## 8. 音声合成（Gemini TTS）仕様

### 8.1 入力
- LLM生成テキスト
- 音声パラメータ（声質/話速等）は本版固定でよい

### 8.2 出力
- 音声バイナリ（mp3 もしくは wav）
- 保存: `temp_audio/` に書き出し

### 8.3 失敗時
- APIキー不備 / ネットワーク / APIエラー → Talkスキップ

#### 8.4 実装方針（Gemini AIライブラリ経由）
- TTS呼び出しは **Gemini AIライブラリ（SDK）** を使用し、SDKが提供するクライアント経由で実行する。
  - 例（Go想定）: `github.com/google/generative-ai-go/genai`
- 直接HTTPで叩く実装は行わない（SDKに寄せて保守性を優先）。

---

## 9. Frontend ⇔ Backend API（Wails バインディング）

### 9.1 データ型（TS想定）

#### `PlayableItem`
```ts
type PlayableKind = "bgm" | "talk" | "silence";

type PlayableItem = {
  id: string;                 // 一意（例: uuid）
  kind: PlayableKind;
  url?: string;               // audio src（silenceは不要）
  mime?: string;              // audio/mpeg 等（任意）
  title: string;
  artist?: string;
  topicTitle?: string;        // talk用
  durationHintMs?: number;    // silence用/推定
  source?: {
    genre?: string;           // bgm
    filePath?: string;        // bgm（デバッグ用、UIには出さない）
    rssUrl?: string;          // talk
    articleUrl?: string;      // talk
  };
};
```

#### `AppConfig`
- `config.json` 相当の型（UIで必要な項目に限定してもよい）

### 9.2 API一覧（案）

#### 設定/初期化
- `LoadConfig(): Promise<AppConfig>`
- `SaveConfig(cfg: AppConfig): Promise<void>`
- `ScanGenres(): Promise<string[]>`
- `ListTracks(genre: string): Promise<{count:number}>`（本版は件数だけでも可）

> 注: LLM設定・Gemini APIキーは **UIから入力し `SaveConfig` で保存** する。環境変数/固定値は前提にしない。

#### 再生
- `GetNextItem(state: { selectedGenre: string }): Promise<PlayableItem>`
  - バックエンドがサイクル状態を保持して「次」を返す
- `SkipCurrent(): Promise<PlayableItem>`
  - 現在の生成/プリフェッチを必要に応じてキャンセルし、次を返す

#### 先読み
- `PrefetchTalk(): Promise<void>`
  - 次がTalkスロットに近いタイミングで呼ぶ（フロント起点でもバック起点でもよい）

### 9.3 状態管理の所在
- サイクルのカウンタ（BGM→Talkの順番）やHistory照合は **バックエンド** で持つ（フロントは単純化）。
- フロントは `selectedGenre` と再生UI状態だけを持つ。

---

## 10. エラーハンドリング/リトライ

### 10.1 方針
- 体験を止めない: **可能な限りBGMへフォールバック**
- UIは致命的なもののみ通知

### 10.2 エラー分類と表示
- 設定不備（BGMルート未設定、ジャンルに曲がない）
  - UI: 明示的に警告し、再生開始を無効化
- ネットワーク/AI生成失敗
  - UI: 通知は任意（トースト程度）
  - 動作: TalkをスキップしてBGM継続

### 10.3 再生エラー時
- `audio.onerror` 発生時:
  - 同一アイテムを再試行しない
  - `GetNextItem()` を呼び、最大連続失敗回数（例: 5）で停止し警告

---

## 11. ログ/デバッグ

### 11.1 ログレベル
- INFO: 再生開始/終了、選曲、Talk生成開始/完了、RSS選定
- WARN: RSS取得失敗、LLM/TTS失敗、スキップ発生
- ERROR: 設定読込/保存失敗、ファイルアクセス不可

### 11.2 個人情報/秘匿
- APIキーはログに出さない
- 記事本文は必要最小限（デバッグ時のみ）

---

## 12. 受け入れ基準（Acceptance Criteria）

1. BGMルート指定後、ジャンル一覧が表示され選択できる
2. 選択ジャンルからBGMがシャッフル再生される
3. BGMを3曲再生した後にTalkが再生される（Talk失敗時はBGMにフォールバックしサイクル継続）
4. RSS複数登録ができ、Talkネタはランダムに選ばれる
5. 同一記事URLは再利用されない（Historyにより重複排除）
6. アプリ終了→起動で設定（APIキー/BGMパス/前回ジャンル/History）が復元される

---

## 13. 実装ToDo（推奨分解）

- Backend
  - 設定/履歴のStore（読み書き、マイグレーション、上限）
  - BGMスキャン（ジャンル列挙、トラック列挙、ランダム選択）
  - RSS取得（複数URL、未使用選定）
  - LLMクライアント（OpenAI互換）
  - TTSクライアント（Gemini AIライブラリ経由）
  - Talk生成パイプライン（RSS→LLM→TTS→temp保存）
  - 音声配信（ローカルHTTP、token化、パス制限）

- Frontend
  - Player View（操作/表示）
  - Settings Modal（CRUD、保存、バリデーション）
  - Audio制御（onended/onerror、スキップ、音量、簡易リトライ）

---

## 14. 未確定事項（実装前に決める）

1. 音声配信方式の詳細: ローカルHTTPサーバのルーティング/認可（token化）/キャッシュ方針
2. LLM設定の必須項目: baseUrl/model/apiKey の必須判定とバリデーション
3. History保持上限（件数/期間）
4. Gemini TTS（SDK）で利用するモデル/音声パラメータ（話速・声質）の固定値

---

## 付録A: RSS初期値
- RSS URLリストは **初期値は空** とし、ユーザーが手動で追加する。
