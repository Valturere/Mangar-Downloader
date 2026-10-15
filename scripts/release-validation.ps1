# Pure validation shared by release preparation/publication and offline tests.
function Assert-MangarReleaseManifest($Manifest, [string]$Version, [string]$Commit, [string]$InstallerName, [long]$InstallerSize, [string]$InstallerSHA256, [string]$ExtensionVersion, [string]$StoreID) {
    $expected = @{ schema=1; version=$Version; channel='stable'; architecture='win-x64'; installer_asset=$InstallerName; installer_size=$InstallerSize; installer_sha256=$InstallerSHA256; extension_version=$ExtensionVersion; extension_store_id=$StoreID; source_commit=$Commit }
    foreach ($key in $expected.Keys) {
        if ($null -eq $Manifest.$key -or [string]$Manifest.$key -cne [string]$expected[$key]) { throw "Manifest field mismatch: $key" }
    }
    $stamp=[DateTimeOffset]::MinValue
    if (![DateTimeOffset]::TryParse($Manifest.created_at, [ref]$stamp) -or $Manifest.created_at -notmatch '^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$') { throw 'Invalid manifest timestamp' }
    if (@($Manifest.PSObject.Properties.Name).Count -ne 11 -or $Commit -cnotmatch '^[a-f0-9]{40}$' -or $InstallerSHA256 -cnotmatch '^[a-f0-9]{64}$' -or $InstallerSize -le 0 -or $InstallerSize -gt 268435456) { throw 'Invalid manifest schema/identity/size' }
}
function Assert-MangarReleaseGates($Record, [string]$Version, [string]$Commit, [string]$InstallerSHA256, [string]$ManifestSHA256, [string]$SumsSHA256) {
    if ($Record.schema -ne 2 -or $Record.version -cne $Version -or $Record.source_commit -cne $Commit) { throw 'Verification record does not match exact source/version' }
    foreach ($key in @('automated','upgrade','equal_version_reinstall','finish_launch','installed_metadata','preservation','native_registration')) {
        if ($Record.$key -cne 'PASS') { throw "Unclosed release gate: $key" }
    }
    foreach ($entry in @(@('installer_sha256',$InstallerSHA256),@('manifest_sha256',$ManifestSHA256),@('sums_sha256',$SumsSHA256))) {
        if ($Record.($entry[0]) -cne $entry[1]) { throw "Unverified release bytes: $($entry[0])" }
    }
    foreach ($key in @('clean_windows','cws_ask_folder')) {
        if ($Record.$key -cne 'PASS' -and !($Record.owner_approved_deferrals -eq $true -and $Record.$key -cin @('DEFERRED','NOT_VERIFIED'))) { throw "Unapproved deferred gate: $key" }
    }
}
