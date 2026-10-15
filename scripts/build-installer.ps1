param([switch]$SkipBuild,[switch]$FailureFixture,[ValidateSet('replacement','registration')][string]$FailurePoint='replacement')
$ErrorActionPreference='Stop'
$taskRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
Push-Location $taskRoot
try {
    & scripts/sync-version.ps1 -Check
    if (!$SkipBuild) { & scripts/build-production.ps1 }
    $version=(Get-Content internal/buildinfo/VERSION -Raw).Trim()
    $payload=Join-Path $taskRoot 'build/bin/production'
    $hashes=[ordered]@{}
    foreach($name in @('YTDLP-GUI-Mangar.exe','mangar-browser-host.exe','mangar-updater.exe')) { $hashes[$name]=(Get-FileHash -LiteralPath (Join-Path $payload $name)).Hash.ToLowerInvariant() }
    $metadata=[ordered]@{schema=1;product_id='YTDLPGUIMangar';version=$version;architecture='win-x64';source_commit=(git rev-parse HEAD);source_dirty=[bool](git status --porcelain=v1);files=$hashes}
    [IO.File]::WriteAllText((Join-Path $payload 'installation.json'),($metadata|ConvertTo-Json -Depth 8)+"`n",[Text.UTF8Encoding]::new($false))
    $bootstrapper=Join-Path $taskRoot 'build/bin/installer-resources/WebView2Bootstrapper.exe'
    New-Item -ItemType Directory -Force (Split-Path $bootstrapper) | Out-Null
    if(!(Test-Path -LiteralPath $bootstrapper)) { Invoke-WebRequest -Uri 'https://go.microsoft.com/fwlink/p/?LinkId=2124703' -OutFile $bootstrapper }
    $signature=Get-AuthenticodeSignature -LiteralPath $bootstrapper
    if($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch 'O=Microsoft Corporation') { throw 'WebView2 bootstrapper must have a valid Microsoft signature' }
    $makensis=Join-Path ${env:ProgramFiles(x86)} 'NSIS/makensis.exe'
    if(!(Test-Path -LiteralPath $makensis)) { $makensis=(Get-Command makensis -ErrorAction Stop).Source }
    $outputDir=Join-Path $taskRoot "dist/release/$version"
    if($FailureFixture) { $outputDir=Join-Path $taskRoot '.local/installer-fixtures';if($FailurePoint -eq 'registration'){$outputDir=Join-Path $outputDir 'after-registration'} }
    New-Item -ItemType Directory -Force $outputDir | Out-Null
    $output=Join-Path $outputDir "YTDLP-GUI-Mangar-Setup-$version-win-x64.exe"
    # Unicode true controls the installer, not decoding of the UTF-8 NSIS input.
    $arguments=@('/V2','/INPUTCHARSET','UTF8',"/DAPP_VERSION=$version","/DPAYLOAD_DIR=$payload","/DWEBVIEW_BOOTSTRAPPER=$bootstrapper","/DINSTALLER_OUTPUT=$output")
    if($FailureFixture) { if($FailurePoint -eq 'replacement'){$arguments+='/DMANGAR_TEST_FAIL_AFTER_REPLACE'}else{$arguments+='/DMANGAR_TEST_FAIL_AFTER_REGISTRATION'} }
    & $makensis @arguments (Join-Path $taskRoot 'build/windows/installer/project.nsi')
    if($LASTEXITCODE -ne 0) { throw 'NSIS installer build failed' }
    Write-Output "Installer: $output"
} finally { Pop-Location }
