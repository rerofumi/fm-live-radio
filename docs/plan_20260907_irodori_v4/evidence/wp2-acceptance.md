# WP-2 / REQ-02 独立受入

判定: **pass**（WP-2 のみ）。計画全体は後続 WP-3–5 が残るため implementing を維持する。REQ-03 以降や製品 v4 推論対応の合格を意味しない。

実施者: persistent acceptance sub-agent /root/wp2_acceptance。実施日: 2026-09-08 JST。

## 対象版と範囲

- 開始時 jj snapshot: 6a85777c。独立比較時の snapshot: f38399142f9d7b555ed81d35bfe28b11699c4e1f（検証用 evidence の追加を含む）。
- 実装 SHA256: tokenizer.go = 376786b3eb23afab51e6324331fbba2a9e9573712490fb3686d37858eaf0ba87。
- テスト SHA256: tokenizer_test.go = 2e6fd81c982cb6c818ed7521dda6c1ad70bf6afd2b8bb7ccf9a3a9c6449035ba。
- 検証前後でこの2ファイルの hash は不変。可変 jj change ID だけではなく上記 hash と [差分](wp2-acceptance-target.diff) で未コミットの対象を固定。
- 差分基点: WP-2前 snapshot 4573c430。製品差分は tokenizer.go と新規 tokenizer_test.go。90_status の REQ-02実装状態、91_worklog、worker証拠以外に今回の変更はない。既存 WP-1 の大量の未コミット差分は本WPの変更と区別した。
- targeted: tokenizer と直接 consumer の localtts/pipeline、関連 audiofmt/store。API の既存メソッドシグネチャ、pipeline、依存・build設定は変更なし。未変更ONNX graph・frontend全回帰は今回不要。

## 独立 E1

1. mise install: 全定義済みツール導入済み。
2. mise x -- go test ./internal/localtts/... ./internal/audiofmt/... ./internal/store/... -count=1 -v: **pass**。[実行ログ](wp2-acceptance-go-test.log)。
   tokenizer の5テスト関数（既存6文を含む15 fixtures、ASCII長、token長、v3 golden、無効長）を実行。skip 0件。
   localtts/pipeline等の「no test files」は動的推論試験成功としては数えず、compile成功としてのみ使用した。
3. 公式 PretrainedTextTokenizer を直接ロードして比較: **656正常条件で raw ids・全padded ids・全mask完全一致、4無効長条件でエラー一致**。
4. 同じ656正常条件を旧snapshotから抽出した実装に通し、現在の v3 の raw ids・全padded ids・全maskと比較: **656/656一致**。

証拠: [比較結果とhash](wp2-acceptance-parity.json)、[全入力条件](wp2-acceptance-cases.json)、[全expected/actual配列の可逆圧縮データ](wp2-acceptance-parity-full.json.gz)。
圧縮前後の SHA256 は比較結果JSONに保存。サイズ制限を避けるため全配列をgzipで保持し、成功結果だけの要約に置き換えて配列証拠を失うことはしていない。

再実行（repository root、ローカル固定資産と既存 uv環境が前提）:

~~~powershell
mise x -- uv run --directory tools/irodori_export --python 3.11 --no-sync python ../../docs/plan_20260907_irodori_v4/evidence/wp2-acceptance-run.py
mise x -- uv run --directory tools/irodori_export --python 3.11 --no-sync python ../../docs/plan_20260907_irodori_v4/evidence/wp2-acceptance-pack.py
~~~

[独立比較スクリプト](wp2-acceptance-run.py) が旧snapshotの実装と現実装を別packageで実行し、[整理スクリプト](wp2-acceptance-pack.py) が全配列を圧縮する。初回は uv --directory に伴う cwd 誤認で実行前に失敗したが、script location からrootを解決するよう検証専用scriptを修正して上記を成功させた。製品コードの修正はしていない。

## 正解系 provenance と条件

- 公式 wrapper: Aratako/Irodori-TTS revision 8224dafb46d0aba89209a8f905f1cb7e3299d9c1 の irodori_tts/tokenizer.py。
  SHA256 981002bf8f89bad1dd319b7873404bfcb4f2e69503b934c2d44e84f88d551c2d。
  固定 upstream.zip 内の同ファイルとbyte一致を独立確認。archive SHA256 e80f64d4a4b1a4db186003bc8f3416f5ecfc2e32b1b73b8b967465fba5c27c9c は WP-1 manifest と一致。
- モデル revision: Aratako/Irodori-TTS-v4.1-Small 2b28324dc263ed5e6638b3cf3dd94c82ead07b4b。
- tokenizer.json SHA256 6a0734cf21c802169defaffe719bc2ef12bb9d0be37e54b61ed27aa89394723d。
  公式ローカルmodelの資産と製品配置model/irodori-v4.1の資産、WP-1 manifest が一致。
- tokenizer_config.json SHA256 d229a271c64de1a7939d20d3665498e873fa91d5ee2edf135d73ec752cb9c9d3。
- Python 3.11.15 / transformers 5.12.1 / tokenizers 0.22.2 / torch 2.10.0+cu128 / Go 1.26.1 windows/amd64。
  tools/irodori_export/uv.lock SHA256 1f47f490fbf1e45ce70d1b1a503822398010e87579d8fb2bb88c0a1fa8670748。既存固定環境を使用し依存変更なし。
- from_pretrained(local_files_only=True)、公式batch_encodeとencode(add_bos=False)を使用。Go側からexpectedを生成していない。

## 要件・実装・テスト対応

- normalizerなし・Metaspace prepend_scheme=never・split=falseの固定v4資産では先頭metaspaceを付けず、ASCII空白は保持して置換する。Unicode正規化を勝手に追加しない。
- PADはv4=3、v3=4。literal added tokenを分離し、入力中のPADでもactive mask=trueになることを比較。
- 旧6文、空文字、単独/連続空白、tab/CRLF/先頭改行、Unicode空白、全半角、結合文字、制御文字、絵文字・異体字・未収録Unicode、数字・記号、特殊token隣接を検証。
- BOS true/false、length 1/64/256/512、特殊token列と改行列の253–258・511–513 token、およびASCII連続文字を検証。
  BOSを加えた結果が255/256/257になる条件を含み、truncation前後の全配列を比較した。
  決定的seed 20260908の混合文100件もBOS有無で比較。
- 旧v3のfloat32スコア、先頭metaspace、旧byte fallbackと特殊tokenの扱いはlegacy分岐に隔離。
  goldenの正当性を現実装から推定せず、snapshot 4573c430 の旧ソースを直接実行した。
  抽出ソース SHA256 d849aff32fcf0f2f0515e6cad1dbab5b72711ed2b07dc4291e214d7eccf50579 は実装前に観測したhashと一致。
  v3資産 SHA256 955dc1fa623fab38cc92a3f4ee172423ae6d73201c4207569bfdf5626bc733f0。
- EncodePaddedChecked は max_length<=0をエラーにする追加API。既存直接consumerは正値256/64で従来EncodePaddedを使い、その契約に変更なし。
- 単体テストのv3 goldenは1文、BOS無しや全境界のexpected配列は単体テストだけでは網羅不足だが、本独立再生成によりREQ-02の必須範囲を補完した。
- ローカルモデルがない場合、現行テストは t.Fatal で失敗する。skipにはならない。今回実資産をロードして実行済み。先行レビューの相対パス不足懸念は受入担当の階層数の数え違いであり撤回した。

## 判定と保持・未検証

REQ-02: **pass**。旧6文不一致は解消し、指定境界とv3保持を独立実行で確認できた。workerログを独立E1の代用にしていない。旧tokenizer-parity.jsonの6/6不一致は変更前の履歴としてのみ保持する。

先行指摘のlegacy分離と256token境界は解消。worker fixture provenanceの不足は公式ソースarchive照合と独立再生成で補完。未承認の仕様変更・WP範囲逸脱・確認範囲内の回帰なし。

本受入は固定v4.1/v3資産のtokenizer条件を対象とし、任意のHugging Face tokenizer形式の汎用互換を保証しない。実Go/ONNX v4推論・caption512の製品組込・音声品質・性能・core E2EはWP-3–5の未検証範囲。state/Acceptanceは変更せず、leaderが本報告からREQ-02行を更新する。


## 文書反映後の差分受入（2026-09-08）

対象 snapshot: f8c940c50767863762a90d593ca43ad9298521ee。独立比較時 f38399142f9d7b555ed81d35bfe28b11699c4e1f からの差分を確認。追加差分は受入証拠とleaderによる90_status/plan_index/40_specification/60_review_packet/cheatsheet/docs/specification/91_worklogの事実反映のみ。コード・テスト・config・依存の変更なし。

tokenizer.go、tokenizer_test.go、直接consumer pipeline.go、uv.lock、v4/v3 tokenizer資産のSHA256は独立E1時と完全一致。検証対象・条件への影響がないため、上記新規独立E1をすべて保持し、再実行しない。

文書の656正常条件・4無効長・v3保持・skipなし・固定資産限定・WP-3以降未実施の記述は証拠と整合。REQ-02のimplemented/passと全計画implementingは整合。過去WP-1/調査結果は履歴として解釈し、今回の結果と混同していない。重大不整合なし、REQ-02 passを維持する。
