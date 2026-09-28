param([Parameter(Mandatory=$true)][string]$HostBinary)
$ErrorActionPreference = 'Stop'
$root = Join-Path ([IO.Path]::GetTempPath()) ('rc-host-check-' + [guid]::NewGuid())
New-Item -ItemType Directory -Path $root | Out-Null
$fixture = Join-Path $root 'fixture.exe'
$hostProcess = $null
try {
  & rustc --edition=2024 ./fixtures/windows-service-child.rs -o $fixture
  if ($LASTEXITCODE) { throw 'service child fixture compilation failed' }
  $image = [IO.File]::ReadAllBytes($HostBinary)
  $pe = [BitConverter]::ToInt32($image, 0x3c)
  if ([BitConverter]::ToUInt16($image, $pe + 24 + 68) -ne 2) { throw 'service host is not a GUI-subsystem executable' }
  foreach ($mode in 'exit','stop','terminate','recover') {
    $state = Join-Path $root $mode
    New-Item -ItemType Directory $state | Out-Null
    $info = Join-Path $state 'fixture.txt'
    $hostMode = if ($mode -eq 'recover') { '--supervise' } else { '--worker' }
    $arguments = @($hostMode,$state,$fixture,$mode,$info) | ForEach-Object { '"' + $_ + '"' }
    $hostProcess = Start-Process -FilePath $HostBinary -ArgumentList $arguments -WindowStyle Hidden -PassThru
    for ($attempt = 0; $attempt -lt 200 -and !(Test-Path $info); $attempt++) { Start-Sleep -Milliseconds 50 }
    if (!(Test-Path $info)) { throw "fixture did not start ($mode)" }
    $fields = (Get-Content -Raw $info).Trim().Split(' ')
    if ($fields[1] -ne '0') { throw 'the Node has a console window' }
    if ($mode -eq 'recover') {
      Stop-Process -Id ([int]$fields[0]) -Force
      for ($attempt = 0; $attempt -lt 200; $attempt++) {
        Start-Sleep -Milliseconds 50
        $next = (Get-Content -Raw $info).Trim().Split(' ')
        if ($next[0] -ne $fields[0]) { break }
      }
      if ($next[0] -eq $fields[0]) { throw 'supervisor did not recover a crashed Node' }
      if (Get-Process -Id ([int]$fields[2]) -ErrorAction SilentlyContinue) { throw 'crashed Node left a descendant behind' }
      $fields = $next
      [IO.File]::WriteAllText((Join-Path $state 'service.stop'), 'stop')
    }
    if ($mode -eq 'stop') { [IO.File]::WriteAllText((Join-Path $state 'service.stop'), 'stop') }
    if ($mode -eq 'terminate') { Stop-Process -Id $hostProcess.Id -Force }
    if (!$hostProcess.WaitForExit(10000)) { throw "service host did not exit ($mode)" }
    if ($mode -eq 'exit' -and $hostProcess.ExitCode -ne 17) { throw 'host lost Node failure exit status' }
    if ($mode -in 'stop','recover' -and $hostProcess.ExitCode -ne 0) { throw 'explicit stop must succeed without a retry' }
    foreach ($childId in @([int]$fields[0], [int]$fields[2])) {
      for ($attempt = 0; $attempt -lt 100 -and (Get-Process -Id $childId -ErrorAction SilentlyContinue); $attempt++) { Start-Sleep -Milliseconds 50 }
      if (Get-Process -Id $childId -ErrorAction SilentlyContinue) { throw "orphaned process $childId ($mode)" }
    }
    if ($mode -ne 'terminate') {
      $log = Get-Content -Raw (Join-Path $state 'service.log')
      foreach ($required in 'fixture stdout','fixture stderr','Node exited') {
        if (!$log.Contains($required)) { throw "missing log $required ($mode)" }
      }
    }
    $hostProcess.Dispose(); $hostProcess = $null
  }
  Write-Output 'Windowless host, stdout/stderr capture, exit status, explicit stop, and process-tree cleanup passed.'
} finally {
  if ($hostProcess -and !$hostProcess.HasExited) { Stop-Process -Id $hostProcess.Id -Force; $hostProcess.WaitForExit() }
  if ($hostProcess) { $hostProcess.Dispose() }
  # $root is a newly created, fixed-prefix fixture directory under the OS temp directory.
  $resolved = [IO.Path]::GetFullPath($root)
  if (!$resolved.StartsWith([IO.Path]::GetFullPath([IO.Path]::GetTempPath()), [StringComparison]::OrdinalIgnoreCase)) { throw 'unsafe fixture path' }
  Remove-Item -LiteralPath $resolved -Recurse -Force
}
