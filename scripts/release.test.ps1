$ErrorActionPreference='Stop'
. (Join-Path $PSScriptRoot 'release-validation.ps1')
function Reject([scriptblock]$Action) { $failed=$false;try{& $Action}catch{$failed=$true};if(!$failed){throw 'Invalid release metadata was accepted'} }
$commit='a'*40;$sha='b'*64;$manifestSha='c'*64;$sumsSha='d'*64
$manifest=[pscustomobject]@{schema=1;version='1.0.3';channel='stable';architecture='win-x64';installer_asset='YTDLP-GUI-Mangar-Setup-1.0.3-win-x64.exe';installer_size=1234;installer_sha256=$sha;extension_version='1.0.1';extension_store_id='paimennadokhgjdpgilfcebameolopbm';created_at='2026-10-15T10:00:00Z';source_commit=$commit}
function CheckManifest { Assert-MangarReleaseManifest $manifest '1.0.3' $commit 'YTDLP-GUI-Mangar-Setup-1.0.3-win-x64.exe' 1234 $sha '1.0.1' 'paimennadokhgjdpgilfcebameolopbm' }
CheckManifest
foreach($key in @('schema','version','channel','architecture','installer_asset','installer_size','installer_sha256','extension_version','extension_store_id','created_at','source_commit')){$before=$manifest.$key;$manifest.$key='invalid';Reject {CheckManifest};$manifest.$key=$before}
$record=[pscustomobject]@{schema=2;version='1.0.3';source_commit=$commit;installer_sha256=$sha;manifest_sha256=$manifestSha;sums_sha256=$sumsSha;automated='PASS';upgrade='PASS';equal_version_reinstall='PASS';finish_launch='PASS';installed_metadata='PASS';preservation='PASS';native_registration='PASS';clean_windows='DEFERRED';cws_ask_folder='NOT_VERIFIED';owner_approved_deferrals=$true}
function CheckGates { Assert-MangarReleaseGates $record '1.0.3' $commit $sha $manifestSha $sumsSha }
CheckGates
foreach($key in @('schema','version','source_commit','installer_sha256','manifest_sha256','sums_sha256','automated','upgrade','equal_version_reinstall','finish_launch','installed_metadata','preservation','native_registration')){$before=$record.$key;$record.$key='FAIL';Reject {CheckGates};$record.$key=$before}
$record.owner_approved_deferrals=$false;Reject {CheckGates};$record.owner_approved_deferrals=$true
$record.clean_windows='FAIL';Reject {CheckGates};$record.clean_windows='DEFERRED'
$record.cws_ask_folder='FAIL';Reject {CheckGates}
Write-Output 'Release manifest/gate validation: PASS (valid stable, all fields/gates tampering, explicit deferrals, exact hashes)'
