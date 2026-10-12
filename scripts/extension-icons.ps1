# Deterministic size/container exports of the project branding master.
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
Add-Type -AssemblyName System.Drawing
$art = [Drawing.Bitmap]::new((Join-Path $taskRoot 'build/appicon.png'))
function New-BrandBitmap([int]$Width,[int]$Height,[int]$ArtSize,[bool]$Opaque=$false) {
    $pixelFormat=if ($Opaque) { [Drawing.Imaging.PixelFormat]::Format24bppRgb } else { [Drawing.Imaging.PixelFormat]::Format32bppArgb }
    $bitmap=[Drawing.Bitmap]::new($Width,$Height,$pixelFormat)
    $graphics=[Drawing.Graphics]::FromImage($bitmap)
    try {
        $background=if ($Opaque) { [Drawing.Color]::FromArgb(16,28,47) } else { [Drawing.Color]::Transparent }
        $graphics.Clear($background)
        $graphics.InterpolationMode=[Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $graphics.PixelOffsetMode=[Drawing.Drawing2D.PixelOffsetMode]::HighQuality
        $ratio=[Math]::Min($ArtSize/$sourceRect.Width,$ArtSize/$sourceRect.Height)
        $w=[int][Math]::Round($sourceRect.Width*$ratio); $h=[int][Math]::Round($sourceRect.Height*$ratio)
        $destination=[Drawing.Rectangle]::new([int](($Width-$w)/2),[int](($Height-$h)/2),$w,$h)
        $graphics.DrawImage($art,$destination,$sourceRect,[Drawing.GraphicsUnit]::Pixel)
    } finally { $graphics.Dispose() }
    return ,$bitmap
}
try {
    $left = $art.Width; $top = $art.Height; $right = -1; $bottom = -1
    for ($y = 0; $y -lt $art.Height; $y++) {
        for ($x = 0; $x -lt $art.Width; $x++) {
            if ($art.GetPixel($x, $y).A -gt 0) {
                $left = [Math]::Min($left, $x); $top = [Math]::Min($top, $y)
                $right = [Math]::Max($right, $x); $bottom = [Math]::Max($bottom, $y)
            }
        }
    }
    if ($right -lt $left) { throw 'App artwork пуст' }
    $sourceRect = [Drawing.Rectangle]::new($left, $top, $right-$left+1, $bottom-$top+1)
    $icons = Join-Path $taskRoot 'browser-extension/icons'
    $frontend=Join-Path $taskRoot 'frontend/src/assets'
    New-Item -ItemType Directory -Force $icons,$frontend | Out-Null
    foreach ($size in 16,32,48,128) {
        $artSize = if ($size -eq 128) { 96 } else { [int][Math]::Round($size * 0.875) }
        $bmp = New-BrandBitmap $size $size $artSize
        try {
            $bmp.Save((Join-Path $icons "$size.png"), [Drawing.Imaging.ImageFormat]::Png)
        } finally { $bmp.Dispose() }
    }
    $sizes=@(16,24,32,48,64,128,256)
    $frames=@()
    foreach ($size in $sizes) {
        $bitmap=New-BrandBitmap $size $size $size
        $buffer=[IO.MemoryStream]::new()
        try {
            $bitmap.Save($buffer,[Drawing.Imaging.ImageFormat]::Png)
            $frames+=,@($buffer.ToArray())
            if ($size -eq 32) { $bitmap.Save((Join-Path $frontend 'app-icon.png'),[Drawing.Imaging.ImageFormat]::Png) }
        } finally { $buffer.Dispose(); $bitmap.Dispose() }
    }
    $ico=[IO.BinaryWriter]::new([IO.File]::Open((Join-Path $taskRoot 'build/windows/icon.ico'),[IO.FileMode]::Create))
    try {
        $ico.Write([uint16]0); $ico.Write([uint16]1); $ico.Write([uint16]$sizes.Count)
        [uint32]$offset=6+16*$sizes.Count
        for ($i=0; $i -lt $sizes.Count; $i++) {
            $dimension=if ($sizes[$i] -eq 256) { 0 } else { $sizes[$i] }
            $ico.Write([byte]$dimension); $ico.Write([byte]$dimension); $ico.Write([byte]0); $ico.Write([byte]0)
            $ico.Write([uint16]1); $ico.Write([uint16]32); $ico.Write([uint32]$frames[$i].Length); $ico.Write($offset)
            $offset+=$frames[$i].Length
        }
        foreach ($frame in $frames) { $ico.Write([byte[]]$frame) }
    } finally { $ico.Dispose() }
    $version = (Get-Content -LiteralPath (Join-Path $taskRoot 'browser-extension/manifest.json') -Raw | ConvertFrom-Json).version
    if ($version -notmatch '^[0-9]+\.[0-9]+\.[0-9]+$') { throw 'Некорректная extension version' }
    $output = Join-Path $taskRoot "dist/release/$version/store"
    New-Item -ItemType Directory -Force $output | Out-Null
    Copy-Item -LiteralPath (Join-Path $icons '128.png') -Destination (Join-Path $output 'icon-128.png')
    foreach ($canvas in @(@(440,280,160),@(1400,560,320))) {
        $bitmap=New-BrandBitmap $canvas[0] $canvas[1] $canvas[2] $true
        try { $bitmap.Save((Join-Path $output "promo-$($canvas[0])x$($canvas[1]).png"),[Drawing.Imaging.ImageFormat]::Png) } finally { $bitmap.Dispose() }
    }
} finally { $art.Dispose() }
Write-Output 'Windows ICO 16/24/32/48/64/128/256, app header, extension PNG, store icon/promo: project master'
