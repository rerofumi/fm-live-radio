Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$url = "https://github.com/microsoft/onnxruntime/releases/download/v1.26.0/onnxruntime-win-x64-gpu_cuda13-1.26.0.zip"
$destDir = Join-Path $PSScriptRoot "..\\third_party\\onnxruntime-gpu"
$zipFile = Join-Path $destDir "onnxruntime-win-x64-gpu_cuda13-1.26.0.zip"
# The CUDA 13 release asset still extracts into the legacy directory name.
$extractDir = Join-Path $destDir "onnxruntime-win-x64-gpu-1.26.0"
$cudaDepsDir = Join-Path $PSScriptRoot "..\\third_party\\nvidia-cudnn-cu13"

function Ensure-Cuda13RuntimeDeps {
    $cudnnMarker = Join-Path $cudaDepsDir "nvidia\\cudnn\\bin\\cudnn64_9.dll"
    if (Test-Path $cudnnMarker) {
        return
    }
    if (-not (Get-Command uv -ErrorAction SilentlyContinue)) {
        throw "uv is required to install nvidia-cudnn-cu13. Run: uv pip install --target third_party\\nvidia-cudnn-cu13 nvidia-cudnn-cu13==9.23.1.3 --link-mode=copy"
    }
    Write-Host "Installing CUDA 13 runtime dependencies via nvidia-cudnn-cu13"
    & uv pip install --target $cudaDepsDir --link-mode=copy nvidia-cudnn-cu13==9.23.1.3
    if ($LASTEXITCODE -ne 0) {
        throw "uv pip install for nvidia-cudnn-cu13 failed with exit code $LASTEXITCODE"
    }
}

function Copy-RuntimeDllsToOrtLib {
    param(
        [string]$LibDir
    )

    Ensure-Cuda13RuntimeDeps

    $runtimeDlls = Get-ChildItem $cudaDepsDir -Recurse -Filter '*.dll'
    foreach ($dll in $runtimeDlls) {
        Copy-Item -LiteralPath $dll.FullName -Destination (Join-Path $LibDir $dll.Name) -Force
    }
}

if (-not (Test-Path $destDir)) {
    New-Item -ItemType Directory -Force -Path $destDir | Out-Null
}

if (Test-Path $extractDir) {
    Write-Host "ONNX Runtime GPU already exists in $extractDir"
    Copy-RuntimeDllsToOrtLib -LibDir (Join-Path $extractDir "lib")
    exit 0
}

Write-Host "Downloading ONNX Runtime GPU 1.26.0"
Write-Host "From: $url"
Write-Host "To: $zipFile"

if (Test-Path $zipFile) {
    Remove-Item -LiteralPath $zipFile -Force
}

try {
    if (Get-Command curl.exe -ErrorAction SilentlyContinue) {
        & curl.exe -L --fail --output $zipFile $url
        if ($LASTEXITCODE -ne 0) {
            throw "curl.exe download failed with exit code $LASTEXITCODE"
        }
    } else {
        Invoke-WebRequest -Uri $url -OutFile $zipFile -UseBasicParsing
    }

    if (Test-Path $extractDir) {
        Remove-Item -Recurse -Force $extractDir
    }
    Write-Host "Extracting to $destDir"
    Expand-Archive -Path $zipFile -DestinationPath $destDir -Force

    $libDir = Join-Path $extractDir "lib"
    Copy-RuntimeDllsToOrtLib -LibDir $LibDir

    Write-Host "Cleaning up archive"
    Remove-Item $zipFile -Force

    Write-Host "Completed: $extractDir"
} catch {
    if (Test-Path $zipFile) {
        Remove-Item -LiteralPath $zipFile -Force -ErrorAction SilentlyContinue
    }
    if (Test-Path $extractDir) {
        Remove-Item -Recurse -Force $extractDir -ErrorAction SilentlyContinue
    }
    throw
}
