param(
  [string]$Token = $env:RC_ENROLL_TOKEN,
  [string]$Server = $env:RC_URL,
  [switch]$ValidateFunctionsOnly
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Add-Type -AssemblyName System.Net.Http
$Api = if ($env:RC_RELEASE_API) { $env:RC_RELEASE_API } else {
  'https://api.github.com/repos/OhRats-Technologies/rc/releases/latest'
}
$Root = if ($env:LOCALAPPDATA) { Join-Path $env:LOCALAPPDATA 'OhRats\RC' } else {
  throw 'LOCALAPPDATA is required'
}
$Bin = if ($env:RC_INSTALL_BIN_DIR) { $env:RC_INSTALL_BIN_DIR } else { Join-Path $Root 'bin' }
$Data = if ($env:RC_DATA_DIR) { $env:RC_DATA_DIR } else { Join-Path $Root 'data' }
$Components = if ($env:RC_COMPONENT_DIR) { $env:RC_COMPONENT_DIR } else { Join-Path $Data 'components' }
$State = if ($env:RC_STATE_DIR) { $env:RC_STATE_DIR } else { Join-Path $Root 'state' }
$Runtime = Join-Path $Data 'runtime'
$Versions = Join-Path $Runtime 'versions'
$Active = Join-Path $Runtime 'active'
$PreviousFile = Join-Path $Runtime 'previous'
$Backup = Join-Path $Runtime 'rollback'
$Journal = Join-Path $Runtime 'activation-journal.json'
$Temp = Join-Path ([IO.Path]::GetTempPath()) ("rc-install-" + [guid]::NewGuid())
$activating = $false; $names = @()
function Protect-PrivateDirectories([string[]]$Paths) {
  $owner = [Security.Principal.WindowsIdentity]::GetCurrent().User; $inherit = [Security.AccessControl.InheritanceFlags]'ContainerInherit,ObjectInherit'
  foreach ($path in $Paths) {
    New-Item -ItemType Directory -Force -Path $path | Out-Null
    $acl = [Security.AccessControl.DirectorySecurity]::new()
    $acl.SetOwner($owner); $acl.SetAccessRuleProtection($true, $false)
    foreach ($sid in $owner,'S-1-5-18','S-1-5-32-544') {
      $identity = if ($sid -is [Security.Principal.SecurityIdentifier]) { $sid } else { [Security.Principal.SecurityIdentifier]::new($sid) }
      $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($identity,[Security.AccessControl.FileSystemRights]::FullControl,$inherit,[Security.AccessControl.PropagationFlags]::None,[Security.AccessControl.AccessControlType]::Allow))
    }
    Set-Acl -LiteralPath $path -AclObject $acl
  }
}
function Download-Limited([string]$Url, [string]$Path, [long]$Limit) {
  if ($Url -notmatch '^https://') { throw "download URL must use HTTPS: $Url" }
  $handler = [Net.Http.HttpClientHandler]::new()
  $handler.AllowAutoRedirect = $true
  $handler.MaxAutomaticRedirections = 5
  $client = [Net.Http.HttpClient]::new($handler)
  $response = $null; $input = $null; $output = $null
  try {
    $response = $client.GetAsync($Url, [Net.Http.HttpCompletionOption]::ResponseHeadersRead).
      GetAwaiter().GetResult()
    $response.EnsureSuccessStatusCode()
    if ($response.RequestMessage.RequestUri.Scheme -ne 'https') { throw "download redirected away from HTTPS: $Url" }
    if ($response.Content.Headers.ContentLength -gt $Limit) { throw "download exceeds limit: $Url" }
    $input = $response.Content.ReadAsStreamAsync().GetAwaiter().GetResult()
    $output = [IO.File]::Create($Path)
    $buffer = [byte[]]::new(65536); [long]$total = 0
    while (($read = $input.Read($buffer, 0, $buffer.Length)) -gt 0) {
      $total += $read
      if ($total -gt $Limit) { throw "download exceeds limit: $Url" }
      $output.Write($buffer, 0, $read)
    }
    $output.Flush($true)
  } catch {
    if (Test-Path -LiteralPath $Path) { Remove-Item -Force -LiteralPath $Path }
    throw
  } finally {
    if ($output) { $output.Dispose() }; if ($input) { $input.Dispose() }
    if ($response) { $response.Dispose() }; $client.Dispose(); $handler.Dispose()
  }
}
function Asset([object]$Release, [string]$Name) {
  $matches = @($Release.assets | Where-Object name -eq $Name)
  if ($matches.Count -ne 1) { throw "release must contain exactly one $Name" }
  $asset = $matches[0]
  if ($asset.browser_download_url -ne "https://github.com/OhRats-Technologies/rc/releases/download/$($Release.tag_name)/$Name") {
    throw "invalid immutable asset URL: $Name"
  }
  if ($asset.digest -notmatch '^sha256:[0-9a-fA-F]{64}$') { throw "missing SHA-256 digest: $Name" }
  $path = Join-Path $Temp $Name
  Download-Limited $asset.browser_download_url $path (160MB)
  $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $path).Hash.ToLowerInvariant()
  if ($actual -ne $asset.digest.Substring(7).ToLowerInvariant()) { throw "checksum mismatch: $Name" }
  $path
}
function Archive-Members([string]$Archive) {
  $members = @(& tar.exe -tzf $Archive)
  if ($LASTEXITCODE -ne 0 -or !$members.Count) { throw "invalid archive: $Archive" }
  $verbose = @(& tar.exe -tvzf $Archive)
  if ($LASTEXITCODE -ne 0 -or $verbose.Count -ne $members.Count) { throw "invalid archive listing: $Archive" }
  foreach ($entry in $verbose) {
    if (!$entry -or ($entry[0] -ne '-' -and $entry[0] -ne 'd')) {
      throw "archive contains a link or special member: $Archive"
    }
  }
  foreach ($member in $members) {
    if ($member.StartsWith('/') -or $member.StartsWith('\') -or $member -match '(^|[\\/])\.\.([\\/]|$)') {
      throw "unsafe archive member: $member"
    }
  }
  $members
}
function Require-Single([string]$Archive, [string]$Member) {
  $members = @(Archive-Members $Archive)
  if ($members.Count -ne 1 -or $members[0] -ne $Member) { throw "archive must contain only $Member" }
}
function Require-RegularBounded([string]$Path, [long]$Limit) {
  $item = Get-Item -Force -LiteralPath $Path
  if ($item.PSIsContainer -or ($item.Attributes -band [IO.FileAttributes]::ReparsePoint) -or
      $item.Length -gt $Limit) { throw "invalid or oversized extracted file: $Path" }
}
function Parse-Kernel-Version([string]$Output) {
  if ($Output -notmatch '^RC kernel ([0-9]+\.[0-9]+\.[0-9]+)$') {
    throw 'RC kernel version is invalid'
  }
  [version]$Matches[1]
}
function Require-Kernel-NotDowngrade([version]$Candidate, [version]$Installed) {
  if ($Candidate -lt $Installed) { throw 'refusing to downgrade RC kernel' }
}
function Atomic-Text([string]$Path, [string]$Value) {
  $temporary = "$Path.new-$PID"
  [IO.File]::WriteAllText($temporary, $Value + "`n", [Text.UTF8Encoding]::new($false))
  if (Test-Path -LiteralPath $Path) { [IO.File]::Replace($temporary, $Path, $null) }
  else { [IO.File]::Move($temporary, $Path) }
}
function Install-Components([string]$Stage, [string[]]$Names) {
  New-Item -ItemType Directory -Force -Path $Components | Out-Null
  foreach ($name in $Names) {
    $source = Join-Path $Stage "components\$name.wasm"
    $target = Join-Path $Components "$name.wasm"
    $marker = Join-Path $Components "$name.core"
    Copy-Item -Force $source "$target.new-$PID"
    Move-Item -Force "$target.new-$PID" $target
    Atomic-Text $marker ('sha256:' + (Get-FileHash -Algorithm SHA256 $target).Hash.ToLowerInvariant())
  }
}
function Restore-Previous([string]$Previous, [string[]]$Names) {
  if ($Previous) { Atomic-Text $Active $Previous }
  elseif (Test-Path $Active) { Remove-Item -Force $Active }
  if (Test-Path (Join-Path $Backup 'rc.exe')) {
    Copy-Item -Force (Join-Path $Backup 'rc.exe') (Join-Path $Bin 'rc.exe')
  }
  foreach ($name in $Names) {
    foreach ($suffix in 'wasm','core') {
      $saved = Join-Path $Backup "components\$name.$suffix"
      $target = Join-Path $Components "$name.$suffix"
      if (Test-Path $saved) { Copy-Item -Force $saved $target }
      elseif (Test-Path $target) { Remove-Item -Force $target }
    }
  }
  $savedVersion = Join-Path $Backup 'installed-version'
  if (Test-Path $savedVersion) {
    Copy-Item -Force $savedVersion (Join-Path $Runtime 'installed-version')
  } elseif (Test-Path (Join-Path $Runtime 'installed-version')) {
    Remove-Item -Force (Join-Path $Runtime 'installed-version')
  }
  if (Test-Path (Join-Path $State 'device.json')) {
    try { & (Join-Path $Bin 'rc.exe') service install } catch {}
  }
}
function Recover-InterruptedActivation {
  if (!(Test-Path $Journal)) { return }
  $record = Get-Content -Raw $Journal | ConvertFrom-Json
  if ($null -eq $record.previous -or $null -eq $record.names) {
    throw 'invalid Windows activation recovery journal'
  }
  Restore-Previous ([string]$record.previous) @($record.names)
  Remove-Item -Force $Journal
}
function Remove-StaleVersions([string]$Current, [string]$Previous) {
  Get-ChildItem -Directory -LiteralPath $Versions | Where-Object { $_.Name -match '^\d+\.\d+\.\d+$' } |
    ForEach-Object {
      if (![string]::Equals($_.FullName,$Current,[StringComparison]::OrdinalIgnoreCase) -and
          ![string]::Equals($_.FullName,$Previous,[StringComparison]::OrdinalIgnoreCase)) { Remove-Item -Recurse -Force $_.FullName }
    }
}
if ($ValidateFunctionsOnly) { return }
New-Item -ItemType Directory -Force -Path $Temp | Out-Null
try {
  Protect-PrivateDirectories @($Root,$Bin,$Data,$Components,$State,$Runtime,$Versions,$Temp)
  Recover-InterruptedActivation
  if ($Api -notmatch '^https://') { throw 'release API must use HTTPS' }
  $releaseFile = Join-Path $Temp 'release.json'
  Download-Limited $Api $releaseFile (4MB)
  $release = Get-Content -Raw $releaseFile | ConvertFrom-Json
  if ($release.tag_name -notmatch '^v([0-9]+\.[0-9]+\.[0-9]+)$') { throw 'invalid release version' }
  $version = $Matches[1]
  $cliArchive = Asset $release 'rc-windows-amd64.tar.gz'
  $kernelArchive = Asset $release 'rc-kernel-windows-amd64.tar.gz'
  $coreArchive = Asset $release 'rc-core-profile.tar.gz'
  Require-Single $cliArchive 'rc.exe'
  Require-Single $kernelArchive 'rc-kernel.exe'
  $coreMembers = @(Archive-Members $coreArchive)
  if ($coreMembers -notcontains 'profile.lock') { throw 'core profile is missing profile.lock' }
  foreach ($member in $coreMembers) {
    if ($member -notmatch '^(profile\.lock|components/?|components/[a-z0-9-]+\.wasm)$') {
      throw "invalid core profile member: $member"
    }
  }
  $stage = Join-Path $Temp 'stage'
  New-Item -ItemType Directory -Force -Path $stage | Out-Null
  & tar.exe -xzf $cliArchive -C $stage
  & tar.exe -xzf $kernelArchive -C $stage
  & tar.exe -xzf $coreArchive -C $stage
  if ($LASTEXITCODE -ne 0) { throw 'could not extract release archives' }
  Require-RegularBounded (Join-Path $stage 'rc.exe') (160MB)
  Require-RegularBounded (Join-Path $stage 'rc-kernel.exe') (160MB)
  Require-RegularBounded (Join-Path $stage 'profile.lock') (4MB)
  $lock = Get-Content -LiteralPath (Join-Path $stage 'profile.lock')
  if ($lock.Count -lt 3 -or $lock[0] -ne 'schema 1' -or $lock[1] -ne 'profile ohrats:core') {
    throw 'invalid core profile lock'
  }
  $names = @()
  foreach ($line in $lock[2..($lock.Count - 1)]) {
    if ($line -notmatch '^component ([a-z0-9-]+) sha256:([0-9a-f]{64})$') { throw 'invalid profile entry' }
    $name = $Matches[1]; $expected = $Matches[2]; $names += $name
    $file = Join-Path $stage "components\$name.wasm"
    Require-RegularBounded $file (48MB)
    if (!(Test-Path $file) -or (Get-FileHash -Algorithm SHA256 $file).Hash.ToLowerInvariant() -ne $expected) {
      throw "component digest mismatch: $name"
    }
  }
  if (($names | Sort-Object -Unique).Count -ne $names.Count) { throw 'duplicate profile component' }
  $archiveNames = @($coreMembers | Where-Object { $_ -match '^components/(.+)\.wasm$' } |
    ForEach-Object { [IO.Path]::GetFileNameWithoutExtension($_) } | Sort-Object)
  if ((Compare-Object ($names | Sort-Object) $archiveNames)) { throw 'profile members do not match lock' }
  if ((& (Join-Path $stage 'rc.exe') version) -ne "RC $version") { throw 'RC version mismatch' }
  $kernelOutput = (& (Join-Path $stage 'rc-kernel.exe') --version)
  if ($LASTEXITCODE -ne 0) { throw 'RC kernel version command failed' }
  $kernelVersion = Parse-Kernel-Version $kernelOutput
  if (Test-Path (Join-Path $Runtime 'installed-version')) {
    $installed = (Get-Content -Raw (Join-Path $Runtime 'installed-version')).Trim()
    if ([version]$version -lt [version]$installed) { throw 'refusing to downgrade RC' }
  }
  if (Test-Path $Active) {
    $activeKernel = Join-Path ((Get-Content -Raw $Active).Trim()) 'rc-kernel.exe'
    if (Test-Path $activeKernel) {
      $activeOutput = (& $activeKernel --version)
      if ($LASTEXITCODE -ne 0) { throw 'installed RC kernel version command failed' }
      Require-Kernel-NotDowngrade $kernelVersion (Parse-Kernel-Version $activeOutput)
    }
  }
  & (Join-Path $stage 'rc-kernel.exe') --component-dir (Join-Path $stage 'components') policy-check | Out-Null
  if ($LASTEXITCODE -ne 0) { throw 'kernel rejected the core profile' }

  $versionDir = Join-Path $Versions $version
  $newDir = "$versionDir.new-$PID"
  if (Test-Path $newDir) { Remove-Item -Recurse -Force $newDir }
  New-Item -ItemType Directory -Force -Path $newDir | Out-Null
  Copy-Item (Join-Path $stage 'rc.exe'),(Join-Path $stage 'rc-kernel.exe') $newDir
  if (!(Test-Path $versionDir)) { Move-Item $newDir $versionDir } else { Remove-Item -Recurse -Force $newDir }
  $previous = if (Test-Path $Active) { (Get-Content -Raw $Active).Trim() } else { '' }
  Atomic-Text $PreviousFile $previous
  $backupNew = "$Backup.new-$PID"
  if (Test-Path $backupNew) { Remove-Item -Recurse -Force $backupNew }
  New-Item -ItemType Directory -Force -Path (Join-Path $backupNew 'components') | Out-Null
  if (Test-Path (Join-Path $Bin 'rc.exe')) { Copy-Item (Join-Path $Bin 'rc.exe') $backupNew }
  if (Test-Path (Join-Path $Runtime 'installed-version')) {
    Copy-Item (Join-Path $Runtime 'installed-version') $backupNew
  }
  foreach ($name in $names) {
    foreach ($suffix in 'wasm','core') {
      $old = Join-Path $Components "$name.$suffix"
      if (Test-Path $old) { Copy-Item $old (Join-Path $backupNew 'components') }
    }
  }
  if (Test-Path $Backup) { Remove-Item -Recurse -Force $Backup }
  Move-Item $backupNew $Backup
  Atomic-Text $Journal (@{previous=$previous;names=$names} | ConvertTo-Json -Compress)
  $activating = $true
  try { & (Join-Path $Bin 'rc.exe') service stop 2>$null } catch {}
  Copy-Item -Force (Join-Path $versionDir 'rc.exe') (Join-Path $Bin "rc.exe.new-$PID")
  Move-Item -Force (Join-Path $Bin "rc.exe.new-$PID") (Join-Path $Bin 'rc.exe')
  Atomic-Text $Active $versionDir
  Install-Components $stage $names
  Atomic-Text (Join-Path $Runtime 'installed-version') $version
  if ($Token) {
    if ($Server) { & (Join-Path $Bin 'rc.exe') enroll $Token --url $Server }
    else { & (Join-Path $Bin 'rc.exe') enroll $Token }
  }
  if (Test-Path (Join-Path $State 'device.json')) { & (Join-Path $Bin 'rc.exe') service install }
  Remove-Item -Force $Journal
  Remove-StaleVersions $versionDir $previous
  $activating = $false
  Write-Host "installed RC $version in $Bin"
} catch {
  if ($activating -and (Test-Path $PreviousFile)) {
    $previous = (Get-Content -Raw $PreviousFile).Trim()
    Restore-Previous $previous $names
    if (Test-Path $Journal) { Remove-Item -Force $Journal }
  }
  throw
} finally {
  if (Test-Path $Temp) { Remove-Item -Recurse -Force $Temp }
}
