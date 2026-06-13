Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$modelDir = Join-Path $PSScriptRoot "..\model\sa3-sm-music"
$files = @{
    "onnx/t5gemma/encoder.onnx" = "https://huggingface.co/stabilityai/stable-audio-3-optimized/resolve/main/onnx/t5gemma/encoder.onnx"
    "onnx/sa3-sm-music/dit_fp16mixed.onnx" = "https://huggingface.co/stabilityai/stable-audio-3-optimized/resolve/main/onnx/sa3-sm-music/dit_fp16mixed.onnx"
    "onnx/same-s/dec_dynamic_bf16.onnx" = "https://huggingface.co/stabilityai/stable-audio-3-optimized/resolve/main/onnx/same-s/dec_dynamic_bf16.onnx"
    "tokenizer/tokenizer.json" = "https://huggingface.co/stabilityai/stable-audio-3-optimized/resolve/main/onnx/tokenizer/tokenizer.json"
}

Write-Host "=========================================================================="
Write-Host "Stable Audio 3 の ONNX モデルをダウンロードします..."
Write-Host "=========================================================================="
Write-Host "注意: このモデルのダウンロードには Stability AI のコミュニティライセンスへの同意が必要です。"
Write-Host "ブラウザで Hugging Face にログインし、あらかじめ同意を行ってください:"
Write-Host "https://huggingface.co/stabilityai/stable-audio-3-optimized"
Write-Host ""

$headers = @{}
if ($env:HF_TOKEN) {
    Write-Host "環境変数 HF_TOKEN が検出されました。トークンを使用してダウンロードします。"
    $headers["Authorization"] = "Bearer $env:HF_TOKEN"
} else {
    Write-Host "HF_TOKEN が設定されていません。ゲートモデルのため、ダウンロードが失敗（403等）する場合は、"
    Write-Host "Hugging Faceでトークン（Read権限）を作成し、環境変数に設定したうえで再実行してください。"
    Write-Host "例:"
    Write-Host "  `$env:HF_TOKEN='your_token_here'"
    Write-Host "  powershell -ExecutionPolicy Bypass -File scripts\download_sa3_models.ps1"
    Write-Host ""
}

foreach ($item in $files.GetEnumerator()) {
    $relPath = $item.Key
    $url = $item.Value
    $destFile = Join-Path $modelDir $relPath
    $destSubdir = Split-Path $destFile

    if (-not (Test-Path $destSubdir)) {
        New-Item -ItemType Directory -Force -Path $destSubdir | Out-Null
    }

    if (Test-Path $destFile) {
        Write-Host "[SKIP] $relPath はすでに存在します。"
        continue
    }

    Write-Host "$relPath をダウンロード中..."
    try {
        if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
            $curlArgs = @("-L", "--fail", "--output", $destFile)
            if ($env:HF_TOKEN) {
                $curlArgs += @("-H", "Authorization: Bearer $env:HF_TOKEN")
            }
            $curlArgs += $url
            & curl.exe @curlArgs
            if ($LASTEXITCODE -ne 0) {
                throw "curl.exe が終了コード $LASTEXITCODE で失敗しました。"
            }
        } else {
            Invoke-WebRequest -Uri $url -OutFile $destFile -Headers $headers -UseBasicParsing
        }
        Write-Host "[SUCCESS] $relPath のダウンロード完了"
    } catch {
        Write-Error "$relPath のダウンロードに失敗しました。ライセンスの同意、または HF_TOKEN の設定が正しいかご確認ください。"
        Write-Host "エラー詳細: $_"
        if (Test-Path $destFile) {
            Remove-Item $destFile -Force
        }
        exit 1
    }
}

Write-Host ""
Write-Host "すべての Stable Audio 3 モデルファイルのダウンロードが完了しました！"
