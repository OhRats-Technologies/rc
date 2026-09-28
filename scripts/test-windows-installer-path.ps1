$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot '../public/install.ps1') -ValidateFunctionsOnly
$savedUser = [Environment]::GetEnvironmentVariable('Path', 'User')
$savedProcess = $env:Path
$Bin = Join-Path $env:TEMP 'RC path fixture\bin'
try {
  Add-BinToPath
  $once = [Environment]::GetEnvironmentVariable('Path', 'User')
  Add-BinToPath
  if ($once -ne [Environment]::GetEnvironmentVariable('Path', 'User')) { throw 'PATH insertion is not idempotent' }
  foreach ($scope in 'User','Process') {
    $actual = [Environment]::GetEnvironmentVariable('Path', $scope)
    if (($actual -split ';') -notcontains $Bin) { throw "$scope PATH does not contain the bin directory" }
  }
  $env:Path = 'C:\Windows;"' + $Bin.ToUpperInvariant() + '\";'
  $quoted = $env:Path
  Add-BinToPath
  if ($env:Path -ne $quoted) { throw 'quoted, case-insensitive duplicate was added' }
  $env:Path = ''
  Add-BinToPath
  if ($env:Path -ne $Bin) { throw 'empty PATH was not handled' }
} finally {
  [Environment]::SetEnvironmentVariable('Path', $savedUser, 'User')
  $env:Path = $savedProcess
}
Write-Output 'Windows installer PATH checks passed.'
