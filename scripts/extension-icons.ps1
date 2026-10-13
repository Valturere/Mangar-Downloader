# Deterministic size/container exports of the project branding master.
param([switch]$ExtensionsOnly)
$ErrorActionPreference = 'Stop'
$taskRoot = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
Add-Type -AssemblyName System.Drawing
$drawingReferences=@('System.Drawing.Common','System.Drawing.Primitives')
foreach ($assemblyName in 'System.Private.Windows.GdiPlus.dll','System.Private.Windows.Core.dll') {
    $drawingAssembly=Join-Path ([IO.Path]::GetDirectoryName([Drawing.Graphics].Assembly.Location)) $assemblyName
    if (Test-Path -LiteralPath $drawingAssembly) { $drawingReferences+=$drawingAssembly }
}
Add-Type -ReferencedAssemblies $drawingReferences -TypeDefinition @'
using System;
using System.Drawing;
using System.Drawing.Drawing2D;
using System.Drawing.Imaging;
public static class MangarBrowserSign {
    public static Bitmap Shrink(Bitmap source, double ratio) {
        int left=source.Width, top=source.Height, right=0, bottom=0;
        for(int y=0;y<source.Height;y++) for(int x=0;x<source.Width;x++) {
            Color c=source.GetPixel(x,y);
            if(c.A>=128 && c.G>100 && c.B>100 && c.G-c.R>50) {
                left=Math.Min(left,x); top=Math.Min(top,y);
                right=Math.Max(right,x); bottom=Math.Max(bottom,y);
            }
        }
        if(left>right) throw new Exception("Browser sign missing");
        // Include the existing antialias/shadow pixels with the sign. Retain
        // the tile and its corner mask; reconstruct only the vacated backdrop
        // from neighbouring pixels of the same original row.
        var region=Rectangle.FromLTRB(left-20,top-20,right+21,bottom+21);
        var result=(Bitmap)source.Clone();
        for(int y=region.Top;y<region.Bottom;y++) {
            Color a=source.GetPixel(region.Left-1,y), b=source.GetPixel(region.Right,y);
            for(int x=region.Left;x<region.Right;x++) {
                double t=(double)(x-region.Left)/region.Width;
                result.SetPixel(x,y,Color.FromArgb(a.A,
                    (int)Math.Round(a.R+(b.R-a.R)*t),
                    (int)Math.Round(a.G+(b.G-a.G)*t),
                    (int)Math.Round(a.B+(b.B-a.B)*t)));
            }
        }
        using(var g=Graphics.FromImage(result)) {
            g.InterpolationMode=InterpolationMode.HighQualityBicubic;
            g.PixelOffsetMode=PixelOffsetMode.HighQuality;
            float w=(float)(region.Width*ratio), h=(float)(region.Height*ratio);
            g.DrawImage(source,new RectangleF((source.Width-w)/2,(source.Height-h)/2,w,h),region,GraphicsUnit.Pixel);
        }
        return result;
    }
}
'@
$art = [Drawing.Bitmap]::new((Join-Path $taskRoot 'build/appicon.png'))
function New-BrandBitmap([int]$Width,[int]$Height,[int]$ArtSize,[bool]$Opaque=$false,[Drawing.Image]$Artwork=$art,[Drawing.Rectangle]$Crop=$sourceRect) {
    $pixelFormat=if ($Opaque) { [Drawing.Imaging.PixelFormat]::Format24bppRgb } else { [Drawing.Imaging.PixelFormat]::Format32bppArgb }
    $bitmap=[Drawing.Bitmap]::new($Width,$Height,$pixelFormat)
    $graphics=[Drawing.Graphics]::FromImage($bitmap)
    try {
        $background=if ($Opaque) { [Drawing.Color]::FromArgb(16,28,47) } else { [Drawing.Color]::Transparent }
        $graphics.Clear($background)
        $graphics.InterpolationMode=[Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
        $graphics.PixelOffsetMode=[Drawing.Drawing2D.PixelOffsetMode]::HighQuality
        $ratio=[Math]::Min($ArtSize/$Crop.Width,$ArtSize/$Crop.Height)
        $w=[int][Math]::Round($Crop.Width*$ratio); $h=[int][Math]::Round($Crop.Height*$ratio)
        $destination=[Drawing.Rectangle]::new([int](($Width-$w)/2),[int](($Height-$h)/2),$w,$h)
        $graphics.DrawImage($Artwork,$destination,$Crop,[Drawing.GraphicsUnit]::Pixel)
    } finally { $graphics.Dispose() }
    return ,$bitmap
}
function New-ExtensionArtwork([Drawing.Rectangle]$Crop) {
    # Crop empty background from the full-resolution master; keep the M/arrow intact.
    $insetX=[int][Math]::Round($Crop.Width*0.025)
    $insetY=[int][Math]::Round($Crop.Height*0.025)
    $cropRect=[Drawing.Rectangle]::new($Crop.X+$insetX,$Crop.Y+$insetY,$Crop.Width-2*$insetX,$Crop.Height-2*$insetY)
    $original=New-BrandBitmap 1024 1024 1024 $false $art $cropRect
    try { $texture=[MangarBrowserSign]::Shrink($original,0.913) }
    finally { $original.Dispose() }
    $bitmap=[Drawing.Bitmap]::new(1024,1024,[Drawing.Imaging.PixelFormat]::Format32bppArgb)
    $graphics=[Drawing.Graphics]::FromImage($bitmap)
    $brush=[Drawing.TextureBrush]::new($texture)
    $path=[Drawing.Drawing2D.GraphicsPath]::new()
    try {
        $diameter=[single](1024*0.12*2)
        $far=[single](1024-$diameter)
        $path.AddArc(0,0,$diameter,$diameter,180,90)
        $path.AddArc($far,0,$diameter,$diameter,270,90)
        $path.AddArc($far,$far,$diameter,$diameter,0,90)
        $path.AddArc(0,$far,$diameter,$diameter,90,90)
        $path.CloseFigure()
        $graphics.Clear([Drawing.Color]::Transparent)
        $graphics.SmoothingMode=[Drawing.Drawing2D.SmoothingMode]::AntiAlias
        $graphics.FillPath($brush,$path)
    } catch { $bitmap.Dispose(); throw }
    finally { $path.Dispose(); $brush.Dispose(); $graphics.Dispose(); $texture.Dispose() }
    return ,$bitmap
}
try {
    $left = $art.Width; $top = $art.Height; $right = -1; $bottom = -1
    $solidLeft = $art.Width; $solidTop = $art.Height; $solidRight = -1; $solidBottom = -1
    for ($y = 0; $y -lt $art.Height; $y++) {
        for ($x = 0; $x -lt $art.Width; $x++) {
            $alpha=$art.GetPixel($x, $y).A
            if ($alpha -gt 0) {
                $left = [Math]::Min($left, $x); $top = [Math]::Min($top, $y)
                $right = [Math]::Max($right, $x); $bottom = [Math]::Max($bottom, $y)
            }
            # Ignore faint stray pixels outside the tile for the browser-only variant.
            if ($alpha -ge 128) {
                $solidLeft = [Math]::Min($solidLeft, $x); $solidTop = [Math]::Min($solidTop, $y)
                $solidRight = [Math]::Max($solidRight, $x); $solidBottom = [Math]::Max($solidBottom, $y)
            }
        }
    }
    if ($right -lt $left) { throw 'App artwork пуст' }
    if ($solidRight -lt $solidLeft) { throw 'Extension artwork пуст' }
    $sourceRect = [Drawing.Rectangle]::new($left, $top, $right-$left+1, $bottom-$top+1)
    $solidRect = [Drawing.Rectangle]::new($solidLeft, $solidTop, $solidRight-$solidLeft+1, $solidBottom-$solidTop+1)
    $extensionArt = New-ExtensionArtwork $solidRect
    $extensionRect = [Drawing.Rectangle]::new(0,0,1024,1024)
    $icons = Join-Path $taskRoot 'browser-extension/icons'
    New-Item -ItemType Directory -Force $icons | Out-Null
    foreach ($size in 16,32,48,128) {
        $bmp = New-BrandBitmap $size $size $size $false $extensionArt $extensionRect
        try {
            $bmp.Save((Join-Path $icons "$size.png"), [Drawing.Imaging.ImageFormat]::Png)
        } finally { $bmp.Dispose() }
    }
    if ($ExtensionsOnly) {
        Write-Output 'Extension PNG 16/32/48/128: browser-specific crop/rounded corners from project master'
        return
    }
    $frontend=Join-Path $taskRoot 'frontend/src/assets'
    New-Item -ItemType Directory -Force $frontend | Out-Null
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
} finally { if ($extensionArt) { $extensionArt.Dispose() }; $art.Dispose() }
Write-Output 'Windows ICO 16/24/32/48/64/128/256, app header, extension PNG, store icon/promo: project master'
