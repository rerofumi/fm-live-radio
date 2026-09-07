# 移行要件と受入条件

state: planned。ここにある数値は将来の受入基準であり、今回の調査で通過したことを意味しない。

| ID | 要件 / 受入条件 | 検証と証拠 | WP |
| --- | --- | --- | --- |
| REQ-01 | v4.1 のモデル・tokenizer・codec・推論ソース・exporter・依存環境を revision/hash で固定し、隔離環境から再作成できる。ライセンス表示と配布対象を記録する | manifest、lock、変換再現コマンド E1/E3 | WP-1 |
| REQ-02 | v4.1 tokenizer の正規化、特殊 token、byte fallback、padding/truncation が公式と一致し、v3 の token 列を変えない | 現在の6文に空白/改行/数字/記号/特殊token/256 token境界を追加。公式 fixtures と ids/mask の完全一致 E1 | WP-2 |
| REQ-03 | v4.1 の全必須 ONNX graph が CPU/CUDA でロード・実行できる。text、caption、speaker、duration、DiT、codec の入出力と型・shape が manifest と一致する | 各 graph の PyTorch/ORT parity、異なる text/ref/latent 長、欠損 external data の明示失敗 E1 | WP-1/3 |
| REQ-04 | speaker と caption を独立に扱い、patch=4 と duration の両条件、null 条件、CFG を公式に整合させる | refのみ / captionのみ / 併用 / 両方なしの fixtures。最小・端数参照長、cfg=1 と既定値で検証 E1/E2 | WP-3 |
| REQ-05 | 現行 narrator で日本語原稿を最後まで生成し、48 kHz mono PCM16、finite、peak>0、RMS>0 の WAV を返す。seconds/durationScale、文間/失敗文の既存動作を維持する | 実 WAV 解析と複数文の統合検証。全無音を成功扱いしただけの結果を除外 E1/E2 | WP-3/4 |
| REQ-06 | v3 設定を保持し、v4.1 を明示選択できる。非対応 manifest、欠損ファイル、hash不一致、誤 tokenizer は推論前に説明付きで拒否する。v3 への復帰が可能 | 旧設定読み込み、新規設定、任意パス、失敗・復帰試験 E1/E2 | WP-4 |
| REQ-07 | CUDA/CPU/auto と共有 ORT/BGM が共存する。取消・停止で実行中 session を破棄しない、後続処理と終了が破綻しない | CUDA 強制失敗、auto fallback、CPU生成、取消後再生成、BGM/Talk 連続運転 E1/E2 | WP-4/5 |
| REQ-08 | 対象 RTX 5090 で200–300字×10原稿を比較し、v4.1 Talk生成のp95がv3の1.2倍以内かつ次のTalk期限に間に合う。20回の定常反復でGPU使用量が増え続けず、最後の5回の範囲が最初の5回の定常中央値+512 MiB以内 | 同条件の初期ロード/各文/全原稿/VRAM/期限を別々に記録 E2。満たさなければ最適化後再計測、既定切替はしない | WP-5 |
| REQ-09 | 10原稿×seed 0/1/2 をv3と比較し、脱落/反復/末尾切れがなく、数字・固有名詞の読み誤り件数がv3以下。声質と自然さを利用者が許容する | 評価用正解読みを作成、同一参照で聴取記録 E4。WAV正常・ASRだけでpassにしない | WP-5 |
| REQ-10 | RSS→原稿→v4.1 WAV→Talk再生→BGM復帰、設定保存/再起動、異常系が通り、ビルドと関連回帰試験が成功。現行docsを実装後に更新する | core E2E、go test、frontend build、mise build、文書差分 E1/E2/E3 | WP-5 |

## 条件の補足

- ONNX FP32 parity の初期基準: 各 graph の同一入力で atol=1e-4、rtol=1e-3。duration の最終 frame 数は公式との差1 frame以内。悪化時に黙って許容値を緩めず、演算別の誤差と音声影響を記録して基準を再検討する。
- 生成ループの parity は seed 値だけで比較しない。Go と PyTorch の乱数列は異なるため、同じ初期 noise、条件 tensor、時刻列、codec 入力を fixtures として渡す。
- アプリの初回範囲は ref のみだが、v4 の統合モデルを正しく変換できた証拠として4条件を graph レベルで検証する。caption の意味的な追従品質は UI 導入時の追加要件。
- CPU の速度は記録するが CUDA と同じ目標は課さない。GPU メモリ測定は PyTorch allocator と nvidia-smi の値を混同しない。
- 試聴用4件は2026-09-07にユーザーが期待通りの品質と確認した（[E4記録](evidence/user-audition.md)）。10原稿×3seedおよび移行後Go/ONNX出力は未評価のため、REQ-09はtodoのまま残し、EX扱いで受入を省略しない。


