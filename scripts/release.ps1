param([switch]$Publish,[switch]$UpdateTest,[switch]$ReleaseCandidate,[string]$GateRecord='')
$ErrorActionPreference='Stop'
$releaseRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
Push-Location $releaseRoot
try {
    $version=(Get-Content internal/buildinfo/VERSION -Raw).Trim()
    if($version -notmatch '^\d+\.\d+\.\d+$'){throw 'Invalid version source'}
    $commit=(git rev-parse HEAD).Trim()
    if($LASTEXITCODE -ne 0){throw 'Cannot read source commit'}
    $sourceStatus=git status --porcelain=v1
    if($LASTEXITCODE -ne 0 -or $sourceStatus){throw 'Release requires clean committed source'}
    if($UpdateTest -and $ReleaseCandidate){throw 'UpdateTest and ReleaseCandidate are mutually exclusive'}
    if($UpdateTest -and $version -notmatch '^0\.9\.\d+$'){throw 'Update-test release is restricted to 0.9.x'}
    if(!$UpdateTest -and !$ReleaseCandidate -and $version -ne '1.0.0'){throw 'First final release must be 1.0.0'}
    if($Publish){
        if(!$GateRecord -or !(Test-Path -LiteralPath $GateRecord -PathType Leaf)){throw 'Publication requires the completed verification record'}
        $gates=Get-Content -LiteralPath $GateRecord -Raw | ConvertFrom-Json
        if($gates.schema -ne 1 -or $gates.source_commit -cne $commit -or $gates.version -cne $version){throw 'Verification record does not match this source/version'}
        foreach($gate in @('baseline','installer','native_launch','first_run_tools','real_mp4','tray_exit','uninstall','preservation')){if($gates.$gate -cne 'PASS'){throw "Unclosed release gate: $gate"}}
        if($ReleaseCandidate -and $gates.real_upgrade -cne 'PASS'){throw 'RC requires verified upgrade'}
        if(!$UpdateTest -and !$ReleaseCandidate){foreach($gate in @('clean_windows','real_upgrade','cws_native','full_final')){if($gates.$gate -cne 'PASS'){throw "Unclosed final gate: $gate"}}}
        $repository=gh repo view Valturere/Mangar-Downloader --json visibility,nameWithOwner | ConvertFrom-Json
        if($LASTEXITCODE -ne 0 -or $repository.visibility -cne 'PUBLIC' -or $repository.nameWithOwner -cne 'Valturere/Mangar-Downloader'){throw 'Release repository must be the approved public repository'}
        $remoteCommit=gh api "repos/Valturere/Mangar-Downloader/commits/$commit" --jq .sha
        if($LASTEXITCODE -ne 0 -or $remoteCommit -cne $commit){throw 'Verified commit must be pushed before creating a Release'}
    }
    if(!$Publish){
        & scripts/build-installer.ps1
        if($LASTEXITCODE -ne 0){throw 'Installer build failed'}
        if(!$ReleaseCandidate){
            & scripts/package-extension.ps1
            if($LASTEXITCODE -ne 0){throw 'Extension archive build failed'}
        }
    }
    $output=Join-Path $releaseRoot "dist/release/$version"
    $installerName="YTDLP-GUI-Mangar-Setup-$version-win-x64.exe"
    $installer=Get-Item -LiteralPath (Join-Path $output $installerName)
    $extension=Get-Content browser-extension/manifest.json -Raw | ConvertFrom-Json
    $store=Get-Content internal/distribution/cws.json -Raw | ConvertFrom-Json
    $sha=(Get-FileHash -LiteralPath $installer.FullName).Hash.ToLowerInvariant()
    if($Publish -and $gates.installer_sha256 -cne $sha){throw 'Installer differs from the verified artifact; do not rebuild after acceptance'}
    $manifest=[ordered]@{schema=1;version=$version;channel='stable';architecture='win-x64';installer_asset=$installerName;installer_size=$installer.Length;installer_sha256=$sha;extension_version=$extension.version;extension_store_id=$store.extension_id;created_at=[DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ');source_commit=$commit}
    if($manifest.extension_store_id -cne 'paimennadokhgjdpgilfcebameolopbm'){throw 'Actual CWS identity mismatch'}
    $manifestFile=Join-Path $output 'Mangar-update.json'
    if($Publish){
        $existing=Get-Content -LiteralPath $manifestFile -Raw | ConvertFrom-Json
        if($existing.source_commit -cne $commit -or $existing.version -cne $version -or $existing.installer_sha256 -cne $sha -or $existing.installer_size -ne $installer.Length -or $existing.extension_store_id -cne $manifest.extension_store_id -or $existing.extension_version -cne $extension.version -or $existing.installer_asset -cne $installerName -or $existing.schema -ne 1 -or $existing.channel -cne 'stable' -or $existing.architecture -cne 'win-x64'){throw 'Verified manifest does not match artifacts'}
    }else{[IO.File]::WriteAllText($manifestFile,($manifest|ConvertTo-Json)+"`n",[Text.UTF8Encoding]::new($false))}
    $zipName=if($ReleaseCandidate){"Mangar-Browser-Extension-$($extension.version)-source-$version-rc.1.zip"}else{"Mangar-Browser-Extension-$($extension.version).zip"}
    $zipSource=Join-Path $releaseRoot "dist/release/$($extension.version)/$zipName"
    $zipTarget=Join-Path $output $zipName
    if($ReleaseCandidate){
        # Original source/key for Load unpacked; do not overwrite the accepted
        # CWS archive or prepare/upload a new Chrome Web Store package.
        if(!$Publish){
            git archive --format=zip --output=$zipTarget HEAD browser-extension
            if($LASTEXITCODE -ne 0){throw 'Unpacked extension source archive failed'}
        }elseif(!(Test-Path -LiteralPath $zipTarget)){throw 'Verified source archive missing'}
    }elseif(![string]::Equals($zipSource,$zipTarget,[StringComparison]::OrdinalIgnoreCase)){Copy-Item -LiteralPath $zipSource -Destination $zipTarget}
    $assets=@($installer.FullName,(Join-Path $output 'Mangar-update.json'),$zipTarget)
    $sums=($assets|ForEach-Object{"$((Get-FileHash -LiteralPath $_).Hash.ToLowerInvariant())  $([IO.Path]::GetFileName($_))"}) -join "`n"
    $sumFile=Join-Path $output 'SHA256SUMS.txt'
    [IO.File]::WriteAllText($sumFile,$sums+"`n",[Text.UTF8Encoding]::new($false))
    $tag=if($ReleaseCandidate){"v$version-rc.1"}else{"v$version"}
    $title=if($UpdateTest){"Тест обновления: Mangar Downloader $version"}elseif($ReleaseCandidate){"Кандидат выпуска: Mangar Downloader $version (RC1)"}else{"Mangar Downloader $version"}
    $notes=@("$title","",$(if($UpdateTest){'Временный public update-test predecessor/target Stage 9. Не финальный accepted release.'}elseif($ReleaseCandidate){'Тестовый pre-release установленного companion app/helper: capability handshake и проверка specialized yt-dlp extractor. Не stable/latest; updater не предлагает prerelease автоматически.'}else{'Первый Windows installer release.'}),"",'Windows10/11 x64, per-user NSIS. Settings/tools/downloads сохраняются при upgrade. Обновления проверяются анонимно; GitHub account/token не нужен.',"",$(if($ReleaseCandidate){'Для тестирования распакуйте source ZIP и загрузите папку browser-extension через Load unpacked. Установленный helper работает без dev-host. Chrome Web Store listing/review/package не обновлялись.'}else{'Расширение устанавливается из Chrome Web Store. ZIP приложен только как архив для разработки, не для обычной установки.'}),"",'Installer пока без Authenticode подписи; поддельный сертификат не используется.',"","Source: $commit","Installer SHA-256: $sha") -join "`n"
    $notesFile=Join-Path $output 'RELEASE_NOTES.md'
    [IO.File]::WriteAllText($notesFile,$notes+"`n",[Text.UTF8Encoding]::new($false))
    if($Publish){
        # No silent tag overwrite or replacement of an existing Release.
        $flags=@();if($ReleaseCandidate){$flags+='--prerelease';$flags+=@('--latest=false')}
        gh release create $tag @assets $sumFile --repo Valturere/Mangar-Downloader --target $commit --title $title --notes-file $notesFile @flags
        if($LASTEXITCODE -ne 0){throw 'GitHub Release publication failed'}
    }
    Write-Output "Release artifacts: $output; publication=$([bool]$Publish)"
} finally {Pop-Location}
