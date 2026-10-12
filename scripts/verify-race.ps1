param([string[]]$Packages = @('.', './internal/browserbridge', './internal/download', './internal/tools', './internal/process', './internal/platform/windows'), [switch]$All, [int]$Count = 1, [string]$Compiler = 'C:/msys64/ucrt64/bin/gcc.exe')
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
if (!(Test-Path -LiteralPath $Compiler -PathType Leaf)) { throw 'Установите MSYS2 UCRT64 и mingw-w64-ucrt-x86_64-gcc; см. build/README.md' }
$oldPath, $oldCGO, $oldCC = $env:PATH, $env:CGO_ENABLED, $env:CC
Push-Location $root
try {
    $env:PATH = (Split-Path $Compiler) + ';' + $env:PATH
    $env:CGO_ENABLED = '1'
    $env:CC = $Compiler
    & $Compiler --version
    $syncLibrary = (& $Compiler --print-file-name libsynchronization.a).Trim()
    if ($LASTEXITCODE -ne 0 -or !(Test-Path -LiteralPath $syncLibrary -PathType Leaf)) { throw 'GCC не поддерживает Windows race detector: libsynchronization.a не найдена' }
    Write-Output "Synchronization library: $syncLibrary"
    go env GOOS GOARCH CGO_ENABLED CC
    if ($All) { $Packages = @('./...') }
    go test -race "-count=$Count" @Packages
    if ($LASTEXITCODE -ne 0) { throw 'Race tests не прошли' }
} finally {
    $env:PATH, $env:CGO_ENABLED, $env:CC = $oldPath, $oldCGO, $oldCC
    Pop-Location
}
