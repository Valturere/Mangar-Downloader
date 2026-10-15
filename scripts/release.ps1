param([switch]$Publish,[switch]$ReleaseCandidate,[ValidateRange(1,999)][int]$RCNumber=1,[string]$GateRecord='')
$ErrorActionPreference='Stop'
$releaseRoot=[IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
. (Join-Path $PSScriptRoot 'release-validation.ps1')
Push-Location $releaseRoot
try {
    & scripts/sync-version.ps1 -Check
    $version=(Get-Content internal/buildinfo/VERSION -Raw).Trim()
    $commit=(git rev-parse HEAD).Trim()
    if($LASTEXITCODE -ne 0 -or $commit -cnotmatch '^[a-f0-9]{40}$'){throw 'Cannot read source commit'}
    $sourceStatus=git status --porcelain=v1
    if($LASTEXITCODE -ne 0 -or $sourceStatus){throw 'Release requires clean committed source'}
    if(!$ReleaseCandidate -and $RCNumber -ne 1){throw 'RCNumber requires explicit ReleaseCandidate'}
    $tag=if($ReleaseCandidate){"v$version-rc.$RCNumber"}else{"v$version"}
    $title=if($ReleaseCandidate){"Предварительный выпуск: Mangar Downloader $version (RC$RCNumber)"}else{"Mangar Downloader $version"}
    if(!$Publish){
        & scripts/build-installer.ps1
        if($LASTEXITCODE -ne 0){throw 'Installer build failed'}
    }
    $output=Join-Path $releaseRoot "dist/release/$version"
    $installerName="YTDLP-GUI-Mangar-Setup-$version-win-x64.exe"
    $installer=Get-Item -LiteralPath (Join-Path $output $installerName)
    $extension=Get-Content browser-extension/manifest.json -Raw | ConvertFrom-Json
    $store=Get-Content internal/distribution/cws.json -Raw | ConvertFrom-Json
    if($store.extension_id -cne 'paimennadokhgjdpgilfcebameolopbm' -or $store.store_version -and $store.store_version -cne $extension.version){throw 'CWS identity/version mismatch'}
    $sha=(Get-FileHash -LiteralPath $installer.FullName).Hash.ToLowerInvariant()
    $manifestFile=Join-Path $output 'Mangar-update.json'
    $sumFile=Join-Path $output 'SHA256SUMS.txt'
    $notesFile=Join-Path $output 'RELEASE_NOTES.md'
    if(!$Publish){
        $manifest=[ordered]@{schema=1;version=$version;channel='stable';architecture='win-x64';installer_asset=$installerName;installer_size=$installer.Length;installer_sha256=$sha;extension_version=$extension.version;extension_store_id=$store.extension_id;created_at=[DateTime]::UtcNow.ToString('yyyy-MM-ddTHH:mm:ssZ');source_commit=$commit}
        [IO.File]::WriteAllText($manifestFile,($manifest|ConvertTo-Json)+"`n",[Text.UTF8Encoding]::new($false))
        $sums=(@($installer.FullName,$manifestFile)|ForEach-Object{"$((Get-FileHash -LiteralPath $_).Hash.ToLowerInvariant())  $([IO.Path]::GetFileName($_))"}) -join "`n"
        [IO.File]::WriteAllText($sumFile,$sums+"`n",[Text.UTF8Encoding]::new($false))
        # Notes are reviewed locally; publication never regenerates accepted bytes.
        $notes=@('### Что нового','','- Добавлена папка загрузок по умолчанию.','- Папку для текущей работы можно временно менять на главном экране.','- Добавлена настройка выбора папки при загрузке из браузерного расширения.','- Исправлен запуск Mangar через «Открыть Mangar Downloader» после установки.','- Подтверждена интеграция с опубликованным расширением Chrome Web Store.','','### Расширение Chrome',"", "Опубликованная Store-версия — $($extension.version). [Установить расширение](https://chromewebstore.google.com/detail/mangar/$($store.extension_id)). Следующее обновление popup и иконок будет выпущено отдельно.",'','### Установка','','Windows 10/11 x64. Установка для текущего пользователя. Настройки, компоненты и готовые файлы сохраняются при обновлении.','','Установщик пока не подписан цифровой подписью; Windows может показать предупреждение.') -join "`n"
        [IO.File]::WriteAllText($notesFile,$notes+"`n",[Text.UTF8Encoding]::new($false))
    }
    $existing=Get-Content -LiteralPath $manifestFile -Raw | ConvertFrom-Json
    Assert-MangarReleaseManifest $existing $version $commit $installerName $installer.Length $sha $extension.version $store.extension_id
    $expectedSums=(@($installer.FullName,$manifestFile)|ForEach-Object{"$((Get-FileHash -LiteralPath $_).Hash.ToLowerInvariant())  $([IO.Path]::GetFileName($_))"}) -join "`n"
    if((Get-Content -LiteralPath $sumFile -Raw) -cne $expectedSums+"`n"){throw 'Checksum file does not match exact release bytes'}
    if($Publish){
        if(!$GateRecord -or !(Test-Path -LiteralPath $GateRecord -PathType Leaf)){throw 'Publication requires the completed verification record'}
        $gates=Get-Content -LiteralPath $GateRecord -Raw | ConvertFrom-Json
        Assert-MangarReleaseGates $gates $version $commit $sha (Get-FileHash $manifestFile).Hash.ToLowerInvariant() (Get-FileHash $sumFile).Hash.ToLowerInvariant()
        if(!(Test-Path -LiteralPath $notesFile -PathType Leaf)){throw 'Reviewed release notes are missing'}
        $repository=gh repo view Valturere/Mangar-Downloader --json visibility,nameWithOwner | ConvertFrom-Json
        if($LASTEXITCODE -ne 0 -or $repository.visibility -cne 'PUBLIC' -or $repository.nameWithOwner -cne 'Valturere/Mangar-Downloader'){throw 'Release repository must be approved and PUBLIC'}
        $remoteCommit=gh api "repos/Valturere/Mangar-Downloader/commits/$commit" --jq .sha
        if($LASTEXITCODE -ne 0 -or $remoteCommit -cne $commit){throw 'Verified commit must be pushed first'}
        $tagCommit=git rev-parse "$tag^{commit}"
        if($LASTEXITCODE -ne 0 -or $tagCommit -cne $commit){throw 'Tag must point to the exact accepted commit'}
        $remoteTag=git ls-remote origin "refs/tags/$tag"
        if($LASTEXITCODE -ne 0 -or ($remoteTag -split '\s+')[0] -cne $commit){throw 'Remote lightweight tag must match the accepted commit'}
        gh release view $tag --repo Valturere/Mangar-Downloader *> $null
        if($LASTEXITCODE -eq 0){throw 'Existing release must not be overwritten'}
        $flags=if($ReleaseCandidate){@('--prerelease','--latest=false')}else{@('--latest')}
        gh release create $tag $installer.FullName $manifestFile $sumFile --repo Valturere/Mangar-Downloader --verify-tag --title $title --notes-file $notesFile @flags
        if($LASTEXITCODE -ne 0){throw 'GitHub Release publication failed'}
    }
    Write-Output "Release artifacts: $output; publication=$([bool]$Publish); tag=$tag"
} finally {Pop-Location}
