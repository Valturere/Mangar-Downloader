#Requires -Version 7.0
$ErrorActionPreference='Stop'
Add-Type -AssemblyName System.Drawing
$root=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$result=@(foreach($size in 16,32,48,128) {
    $image=[Drawing.Bitmap]::new((Join-Path $root "browser-extension/icons/$size.png"))
    try {
        if($image.Width -ne $size -or $image.Height -ne $size){throw 'PNG dimensions mismatch'}
        $left=$size;$top=$size;$right=0;$bottom=0
        for($y=0;$y -lt $size;$y++){for($x=0;$x -lt $size;$x++){
            $c=$image.GetPixel($x,$y)
            if($c.A -ge 128 -and $c.G -gt 100 -and $c.B -gt 100 -and ($c.G-$c.R) -gt 50){
                $left=[Math]::Min($left,$x);$top=[Math]::Min($top,$y);$right=[Math]::Max($right,$x+1);$bottom=[Math]::Max($bottom,$y+1)
            }
        }}
        if($size -eq 128 -and (($right-$left) -lt 82 -or ($right-$left) -gt 86)){throw 'Browser mark width outside 82–86 px'}
        # The 16px downsample includes an antialiased corner pixel.
        if($image.GetPixel(0,0).A -ge 128){throw 'Rounded icon tile corner lost'}
        [pscustomobject]@{canvas=$size;bbox=@($left,$top,$right,$bottom);width=($right-$left);height=($bottom-$top)}
    } finally {$image.Dispose()}
})
$result|ConvertTo-Json -Depth 4
