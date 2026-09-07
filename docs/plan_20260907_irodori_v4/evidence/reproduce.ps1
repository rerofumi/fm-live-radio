param(
    [ValidateSet('Setup','Tokenizer','Baseline','V4','V41','Exporter','Validate')]
    [string]$Action = 'Validate'
)
$ErrorActionPreference = 'Stop'
$repoRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../..'))
Set-Location -LiteralPath $repoRoot
$work = Join-Path $repoRoot 'third_party/irodori-v4-research'
$sourceRevision = '8224dafb46d0aba89209a8f905f1cb7e3299d9c1'
$exporterRevision = '5df35d8720f810902971745a5ad961ff436bd73c'
$source = Join-Path $work "Irodori-TTS-$sourceRevision"
$env:PYTHONIOENCODING = 'utf-8'
$env:PYTHONUTF8 = '1'
if ($Action -eq 'Setup') {
    mise install
    if ($LASTEXITCODE -ne 0) { throw 'mise install failed' }
    New-Item -ItemType Directory -Force $work | Out-Null
    foreach ($item in @(
        @{repo='Aratako/Irodori-TTS';rev=$sourceRevision;zip='upstream.zip'},
        @{repo='mtsmfm/Irodori-TTS-ONNX';rev=$exporterRevision;zip='exporter.zip'}
    )) {
        $archive=Join-Path $work $item.zip
        Invoke-WebRequest "https://github.com/$($item.repo)/archive/$($item.rev).zip" -OutFile $archive
        Expand-Archive -LiteralPath $archive -DestinationPath $work -Force
    }
    foreach ($item in @(
        @{model='Irodori-TTS-v4-Small';rev='4c92c7ee2bb15c19a97cf4e86d24fd6bf33b0135'},
        @{model='Irodori-TTS-v4.1-Small';rev='2b28324dc263ed5e6638b3cf3dd94c82ead07b4b'}
    )) {
        @{sha=$item.rev} | ConvertTo-Json | Set-Content (Join-Path $work "$($item.model)-info.json") -Encoding utf8
        Invoke-WebRequest "https://huggingface.co/Aratako/$($item.model)/resolve/$($item.rev)/tokenizer/tokenizer.json" -OutFile (Join-Path $work "$($item.model)-tokenizer.json")
    }
    mise x -- uv sync --project $source --locked --extra cu128 --python 3.11
} elseif ($Action -eq 'Tokenizer') {
    mise x -- go run './docs/plan_20260907_irodori_v4/evidence/go_probe' tokenizer (Join-Path $work 'Irodori-TTS-v4.1-Small-tokenizer.json') | Set-Content (Join-Path $PSScriptRoot 'go-tokenizer.json') -Encoding utf8
} elseif ($Action -eq 'Baseline') {
    mise x -- go run './docs/plan_20260907_irodori_v4/evidence/go_probe'
} elseif ($Action -in @('V4','V41')) {
    $version = if ($Action -eq 'V4') { 'v4' } else { 'v4.1' }
    mise x -- uv run --project $source --python 3.11 --no-sync python (Join-Path $PSScriptRoot 'runtime_probe.py') --version $version
} elseif ($Action -eq 'Exporter') {
    mise x -- uv run --project $source --python 3.11 --no-sync python (Join-Path $PSScriptRoot 'exporter_probe.py')
} else {
    mise x -- uv run --project $source --python 3.11 --no-sync python (Join-Path $PSScriptRoot 'validate_wavs.py')
}
if ($LASTEXITCODE -ne 0) { throw "$Action failed with exit code $LASTEXITCODE" }
