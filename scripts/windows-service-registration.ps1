# This account has no desktop login session: validate task ownership and lifecycle
# permissions here; the logged-in runner exercises actual execution separately.
$ErrorActionPreference = 'Stop'
$scheduler = New-Object -ComObject 'Schedule.Service'
$scheduler.Connect()
$folder = $scheduler.GetFolder('\')
if (@($folder.GetTasks(0) | Where-Object Name -eq 'OhRats RC Node').Count) {
  throw 'refusing to replace an existing RC task during registration checks'
}
$env:RC_STATE_DIR = $State
$env:RC_COMPONENT_DIR = $Components
$env:RC_KERNEL = (Resolve-Path './kernel/target/debug/rc-kernel.exe').Path
$binary = (Resolve-Path './target/debug/rc.exe').Path
# Offline synthetic identity, never sent to a server.
$seed = 'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'
$device = @{v=1;deviceId='registration-fixture';identitySeed=$seed;transportSecret=$seed}
[IO.File]::WriteAllText((Join-Path $State 'device.json'), ($device | ConvertTo-Json))
[IO.File]::WriteAllText((Join-Path $State 'config.json'), '{"server":"http://127.0.0.1:1"}')
$owner = [Security.Principal.WindowsIdentity]::GetCurrent().User
foreach ($name in 'device.json','config.json') {
  $acl = [Security.AccessControl.FileSecurity]::new()
  $acl.SetOwner($owner)
  $acl.SetAccessRuleProtection($true, $false)
  $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($owner, 'FullControl', 'Allow'))
  [IO.File]::SetAccessControl((Join-Path $State $name), $acl)
}
try {
  foreach ($action in 'install','status','stop','start') {
    & $binary service $action
    if ($LASTEXITCODE) { throw "non-admin service $action failed" }
  }
  $definition = $folder.GetTask('OhRats RC Node').Definition
  $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
  foreach ($userId in $definition.Principal.UserId,$definition.Triggers.Item(1).UserId) {
    if (!$userId) { throw 'service principal and logon trigger require an explicit user' }
    $resolved = if ($userId.StartsWith('S-1-')) { $userId } else {
      [Security.Principal.NTAccount]::new($userId).Translate([Security.Principal.SecurityIdentifier]).Value
    }
    if ($resolved -ne $sid) { throw 'service principal and logon trigger must both belong to the current user' }
  }
  if ($definition.Principal.RunLevel -ne 0 -or $definition.Principal.LogonType -ne 3) {
    throw 'service must use a limited interactive token'
  }
  if ($definition.Settings.ExecutionTimeLimit -ne 'PT0S' -or
      $definition.Settings.DisallowStartIfOnBatteries -or $definition.Settings.StopIfGoingOnBatteries) {
    throw 'service must allow unlimited runtime and battery operation'
  }
  if ($definition.Actions.Item(1).Path -ne $env:RC_KERNEL) { throw 'wrong service executable' }
  if (!$definition.Actions.Item(1).Arguments.Contains($State)) { throw 'service lost its state directory' }
} finally {
  & $binary service uninstall
  if ($LASTEXITCODE) { throw 'non-admin service uninstall failed' }
}
Write-Output 'Non-administrator service registration, replacement, status, stop and removal passed.'
