# Irodori v4.1 ONNX exporter

このディレクトリは製品の実行時依存から分離した v4.1-small の変換環境です。公式
Irodori-TTS の指定 revision と、checkpoint に格納された ModernBERT の重みを使って
text/caption、speaker、duration、DiT step、DACVAE codec の graph を生成します。

## 再現

リポジトリルートで次を実行します（Windows PowerShell）。

```powershell
Push-Location tools/irodori_export
mise install
Pop-Location
mise x -- uv sync --project tools/irodori_export --locked --python 3.11
$output = 'model/irodori-v4.1-rebuild'
if (Test-Path $output) { throw 'output directory must be new and empty' }
New-Item -ItemType Directory -Path $output | Out-Null
mise x -- uv run --project tools/irodori_export --python 3.11 --no-sync python tools/irodori_export/export.py --device cpu --output model/irodori-v4.1-rebuild
mise x -- uv run --project tools/irodori_export --python 3.11 --no-sync python tools/irodori_export/externalize.py --model-dir model/irodori-v4.1-rebuild
mise x -- uv run --project tools/irodori_export --python 3.11 --no-sync python tools/irodori_export/merge_manifest.py --model-dir model/irodori-v4.1-rebuild
mise x -- uv run --project tools/irodori_export --python 3.11 --no-sync python tools/irodori_export/parity.py --model-dir model/irodori-v4.1-rebuild --device cpu
```

この exporter 専用の `tools/irodori_export/mise.toml` は Python `3.11.15` と uv
`0.12.9` を固定します。`uv.lock` は Python 3.11 系、PyTorch CUDA 12.8 wheel、
ONNX Runtime GPU、DACVAE code revision `414c20785fc3a28373073ea8ef7a1316eeeaca6e`
を固定するため、同期は必ず `--locked` で行います。

source を作り直す場合（通常はリポジトリ内の固定 tree を使います）：

```powershell
$archive = 'third_party/irodori-v4-research/upstream.zip'
Invoke-WebRequest 'https://github.com/Aratako/Irodori-TTS/archive/8224dafb46d0aba89209a8f905f1cb7e3299d9c1.zip' -OutFile $archive
$expected = 'e80f64d4a4b1a4db186003bc8f3416f5ecfc2e32b1b73b8b967465fba5c27c9c'
if ((Get-FileHash $archive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $expected) { throw 'upstream.zip SHA256 mismatch' }
Expand-Archive -LiteralPath $archive -DestinationPath 'third_party/irodori-v4-research' -Force
```

固定 source archive は `third_party/irodori-v4-research/upstream.zip` です。
archive の SHA256 は
`e80f64d4a4b1a4db186003bc8f3416f5ecfc2e32b1b73b8b967465fba5c27c9c`、展開後の
source tree hash は
`f159d56bdbde0e6ada8d8fec875a4c66c540baff23425fe661ae000f6a7b437f` で、両方を
export 前に照合します。checkpoint と tokenizer は固定配布物を配置し、次の
hash と revision を照合してください。

```powershell
Get-FileHash third_party/irodori-v4-research/Irodori-TTS-v4.1-Small/model.safetensors -Algorithm SHA256
# c85de88c01700cb53538e706f128ebcb1b8513ad21d7d0e75f58bc82cdbf89f6
Get-FileHash third_party/irodori-v4-research/Irodori-TTS-v4.1-Small/tokenizer/tokenizer.json -Algorithm SHA256
# 6a0734cf21c802169defaffe719bc2ef12bb9d0be37e54b61ed27aa89394723d
```

新しい環境での取得元は checkpoint `Aratako/Irodori-TTS-v4.1-Small` revision
`2b28324dc263ed5e6638b3cf3dd94c82ead07b4b`、tokenizer `sbintuitions/modernbert-ja-310m`
revision `77675fc96a7e445e982e2ba90246b816efc74ec6` です。既存の固定環境から
取得する場合は、次のコマンドで同じ `huggingface_hub` を使えます。

```powershell
mise x -- uv run --project tools/irodori_export --locked python -c "from huggingface_hub import snapshot_download; snapshot_download(repo_id='Aratako/Irodori-TTS-v4.1-Small', revision='2b28324dc263ed5e6638b3cf3dd94c82ead07b4b', local_dir='third_party/irodori-v4-research/Irodori-TTS-v4.1-Small', allow_patterns=['model.safetensors','tokenizer/*'])"
```

codec は `Aratako/Semantic-DACVAE-Japanese-32dim` の weights revision
`47376ee24834d7a05a48ebabfe3cde29b3c5e214` を export 側が明示的に取得し、
revision を指定できない公式 loader には解決済みの local `weights.pth` を渡します。
weights file SHA256 は
`db120339c5ee7eca1912cdf29bc612b947a0808e69c3cebfb4936b45a762c1d5` です。
`--codec <local-dir-or-weights.pth>` も許可しますが、この hash が一致しなければ
直ちに失敗します。

export をやり直すときは、既存の `.onnx.data` を再利用しないよう、必ず空の出力
ディレクトリを指定してください。外部 data の再配置や上書きは既存ファイルを
append することがあるため、既存の `model/irodori-v4.1` を直接指定しません。
例えば `New-Item -ItemType Directory model/irodori-v4.1-rebuild` を作り、
`--output model/irodori-v4.1-rebuild` を付けて実行します。parity と merge は
完成した6 graphのディレクトリに対して実行します。

`uv.lock` は exporter の依存パッケージと DACVAE source の
`414c20785fc3a28373073ea8ef7a1316eeeaca6e` を固定します。export 前に固定
checkpoint/tokenizer の SHA256 を検証します。固定 source tree は
`Irodori-TTS-8224dafb46d0aba89209a8f905f1cb7e3299d9c1` です。この revision
で取得し、source hash 検証対象のディレクトリ名を維持します。
parity は PyTorch/ORT の TF32 を無効にして CPU/CUDA の両 provider を実行し、
動的長、公式の floor/truncate と speaker patch=4 境界、4 条件、external data
削除、自然文 40 step リクエストを検証します。tokenizer・duration feature・
Euler 算術は固定公式 Python utility、ニューラル段階はすべて ONNX Runtime
です。graph、codec、external data、provider、smoke のいずれかが欠けるか
失敗した場合は非ゼロ終了します。生成 report には graph I/O/hash、bundle
サイズ、RSS/VRAM、入力 manifest の hash を記録します。

codec は 48 kHz、hop 1920 samples です。report では waveform 長
1919/1920/1921 と 479999/480000/480001、および narrator の 10 秒全長も
追加検証します。CUDA profiling には graph node を実行した provider を記録
します（shape helper が CPU になる記録は想定内です）。

この WP の配布対象は model directory 内の 6 ONNX graph、各 `.onnx.data`
sidecar、`tokenizer/tokenizer.json` です。exporter の仮想環境と開発用依存は
開発専用です。graph/model は MIT、codec code は Apache-2.0、tokenizer は ModernBERT-ja checkpoint
由来の MIT 表示（Copyright (c) 2025 SB Intuitions）として扱い、各配布対象の SPDX ID と
upstream license/NOTICE URL は
`model/irodori-v4.1/manifest.json` に記録します。
固定revisionから取得したModernBERTとDACVAEのlicense原文も
`docs/licenses/irodori/` に保存し、manifestの
`licenses.notice_files` にSHA256を記録します。これは配布対象へ依存環境を追加する
ものではありません。

入力 checkpoint の SHA256、公式 source revision/archive、tokenizer、codec、Python/uv lock は
`model/irodori-v4.1/manifest.json` に保存されます。初回の codec 解決は Hugging Face
へのアクセスが必要です。`--device cpu` または `--device cuda` を指定できます。

初期の `official_smoke` 証跡には公式 `InferenceRuntime.synthesize`（PyTorch）を
oracle として呼んだものがあり、これは ORT 生成の証明ではありません。最終証跡の
`docs/plan_20260907_irodori_v4/evidence/parity-worker-ort-final.json` は、実テキスト
から text/speaker/duration/DiT 40 step/codec encode/decode の各ニューラル段階を
ORT で実行し、同一入力を公式 PyTorch wrapper に渡した比較を別途記録しています。

変換失敗は manifest の `export.errors` と標準エラーに、演算・shape・precisionを
含めて記録します。失敗を成功として扱う互換 fallback はありません。

## Go parity fixture の再生成と配布境界

`model/irodori-v4.1/go-parity-fixture.json` は公式 PyTorch wrapper の値を保存した
検証専用 fixture です。`manifest.json` の `fixtures.sha256` と、fixture 内の
  `provenance`（source/model/tokenizer/codec revision、各ファイル hash、`uv.lock` hash、
  generator hash）を同時に更新します。製品の起動・通常生成はこの fixture や Python
  環境を要求しません。

  duration predictor の条件行には、公式 `build_duration_features` の入力テキスト
  （fixture 共通の `duration_feature_text`）、token count、max text length、speaker/caption
  flags と、公式14要素の期待ベクトル `duration_features` を保存します。Go parity は
  期待ベクトルをNN入力へ転送せず、製品の `buildDurationFeatures` で再構成した14要素を
  `atol=1e-4`/`rtol=1e-3` で比較してから、その再構成値をduration predictorへ渡します。

固定入力を明示し、空の出力先から再生成します。codec は revision 固定の weights を
解決し、hash が違う場合は生成前に失敗します。

```powershell
$out = 'model/irodori-v4.1-go-parity-rebuild'
if (Test-Path $out) { throw 'output directory must be new and empty' }
New-Item -ItemType Directory $out | Out-Null
mise x -- uv run --project tools/irodori_export --locked python tools/irodori_export/generate_go_fixture.py `
  --source third_party/irodori-v4-research/Irodori-TTS-8224dafb46d0aba89209a8f905f1cb7e3299d9c1 `
  --checkpoint third_party/irodori-v4-research/Irodori-TTS-v4.1-Small/model.safetensors `
  --tokenizer third_party/irodori-v4-research/Irodori-TTS-v4.1-Small/tokenizer/tokenizer.json `
  --codec Aratako/Semantic-DACVAE-Japanese-32dim `
  --out "$out/go-parity-fixture.json"
```

`model/`、`third_party/`、exporter の `.venv` はリポジトリの ignore 対象です。
配布 bundle に含めるのは manifest が示す6 graph、`.onnx.data`、tokenizer と license
notice だけで、固定 source、checkpoint、codec weights、lock、fixture は再現・受入
用の開発/検証資産です。fixture を配布 bundle に同梱したり、製品の起動時に必須化
したりしてはいけません。

