# 移行要件と受入条件

状態の正本は [90_status.md](90_status.md)。ここにある数値は受入基準であり、過去の公式Python調査だけで通過したことを意味しない。WP-1のONNX検証結果とWP-2以降の製品受入を区別する。

| ID | 要件 / 受入条件 | 検証と証拠 | WP |
| --- | --- | --- | --- |
| REQ-01 | v4.1 のモデル・tokenizer・codec・推論ソース・exporter・依存環境を revision/hash で固定し、隔離環境から再作成できる。ライセンス表示と配布対象を記録する | manifest、lock、変換再現コマンド E1/E3 | WP-1 |
| REQ-02 | v4.1 tokenizer の正規化、特殊 token、byte fallback、padding/truncation が公式と一致し、v3 の token 列を変えない | 現在の6文に空白/改行/数字/記号/特殊token/256 token境界を追加。公式 fixtures と ids/mask の完全一致 E1 | WP-2 |
| REQ-03 | v4.1 の全必須 ONNX graph が CPU/CUDA でロード・実行できる。text、caption、speaker、duration、DiT、codec の入出力と型・shape が manifest と一致する | 各 graph の PyTorch/ORT parity、異なる text/ref/latent 長、欠損 external data の明示失敗 E1 | WP-1/3 |
| REQ-04 | speaker と caption を独立に扱い、patch=4 と duration の両条件、null 条件、CFG を公式に整合させる | refのみ / captionのみ / 併用 / 両方なしの fixtures。最小・端数参照長、cfg=1 と既定値で検証 E1/E2 | WP-3 |
| REQ-05 | 現行 narrator で日本語原稿を最後まで生成し、48 kHz mono PCM16、finite、peak>0、RMS>0 の WAV を返す。seconds/durationScale、文間/失敗文の既存動作を維持する | 実 WAV 解析と複数文の統合検証。全無音を成功扱いしただけの結果を除外 E1/E2 | WP-3/4 |
| REQ-06 | v3 設定を保持し、v4.1 を明示選択できる。非対応 manifest、欠損ファイル、hash不一致、誤 tokenizer は推論前に説明付きで拒否する。v3 への復帰が可能 | 旧設定読み込み、新規設定、任意パス、失敗・復帰試験 E1/E2 | WP-4 |
| REQ-07 | CUDA/CPU/auto と共有 ORT/BGM が共存する。取消・停止で実行中 session を破棄しない、後続処理と終了が破綻しない | CUDA 強制失敗、auto fallback、CPU生成、取消後再生成、BGM/Talk 連続運転 E1/E2 | WP-4/5 |
| REQ-08 | 対象RTX5090で40step・200–300字×10原稿と20定常反復を正常生成し、v4.1 Talk生成が次のTalk期限60秒内に収まる。p95 v3/v4比とprocess VRAMを条件・欠測理由付きで記録する。v3同等資源、比1.2、+512MiBは採用条件にしない | 正式時間/生成/条件/版記録 E2。VRAM欠測はunavailableのまま診断保存。利用者採用判断は [承認](evidence/user-acceptance-v41.md) | WP-5 |
| REQ-09 | 10原稿×seed0/1/2の正式音声を用意し、聞き取れるニュースアナウンスとして声質・読み・自然さを利用者が許容する。v3への誤読件数の優位性や無誤読を採用条件にしない | 正式60 WAV/正解読みを提示し、利用者の総合許容判断 E4を記録。全項目採点済みとは推測しない | WP-5 |
| REQ-10 | RSS→原稿→v4.1 WAV→Talk再生→BGM復帰、設定保存/再起動、異常系が通り、ビルドと関連回帰試験が成功。現行docsを実装後に更新する | core E2E、go test、frontend build、mise build、文書差分 E1/E2/E3 | WP-5 |

## 条件の補足

- ONNX FP32 parity の初期基準: 各 graph の同一入力で atol=1e-4、rtol=1e-3。duration の最終 frame 数は公式との差1 frame以内。悪化時に黙って許容値を緩めず、演算別の誤差と音声影響を記録して基準を再検討する。
- 生成ループの parity は seed 値だけで比較しない。Go と PyTorch の乱数列は異なるため、同じ初期 noise、条件 tensor、時刻列、codec 入力を fixtures として渡す。
- アプリの初回範囲は ref のみだが、v4 の統合モデルを正しく変換できた証拠として4条件を graph レベルで検証する。caption の意味的な追従品質は UI 導入時の追加要件。
- CPU の速度は記録するが CUDA と同じ目標は課さない。GPU メモリ測定は PyTorch allocator と nvidia-smi の値を混同しない。
- 試聴用4件は2026-09-07にユーザーが期待通りの品質と確認した（[E4記録](evidence/user-audition.md)）。その後の正式60 WAVと2026-09-08の[利用者回答](evidence/user-acceptance-v41.md)による品質許容を、改訂REQ-09のE4として扱う。旧4件確認だけから全件採点済みとは推測しない。




## 2026-09-08 改訂の優先関係

[利用者採用判断](evidence/user-acceptance-v41.md)に基づきREQ-08/09を改訂した。旧性能gateの未達結果は診断履歴として保持する。生成失敗、60秒期限、provenance、件数・seed等の検証は維持する。REQ-10の未観測UI操作は記録を残すが、モデル採用・新規既定切替とは分離する。

