$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repository = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$name = 'rc-e2e-' + [guid]::NewGuid().ToString('N').Substring(0, 8)
$password = [guid]::NewGuid().ToString('N') + 'aA1!'
Write-Output "::add-mask::$password"
$secure = ConvertTo-SecureString $password -AsPlainText -Force
$fixture = Join-Path $env:PUBLIC $name
$child = $null; $user = $null
try {
  $user = New-LocalUser -Name $name -Password $secure -AccountNeverExpires -PasswordNeverExpires
  Add-LocalGroupMember -SID 'S-1-5-32-545' -Member $user
  New-Item -ItemType Directory -Path $fixture | Out-Null
  $acl = [Security.AccessControl.DirectorySecurity]::new()
  $acl.SetAccessRuleProtection($true, $false)
  foreach ($sid in $user.SID.Value,'S-1-5-18','S-1-5-32-544') {
    $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new(
      [Security.Principal.SecurityIdentifier]::new($sid), 'FullControl',
      'ContainerInherit,ObjectInherit', 'None', 'Allow'))
  }
  [IO.FileSystemAclExtensions]::SetAccessControl([IO.DirectoryInfo]::new($fixture), $acl)
  # Bun is installed in the runner's private profile; give the test user its own copy.
  Copy-Item (Get-Command bun.exe).Source (Join-Path $fixture 'bun.exe')
  $worker = Join-Path $fixture 'worker.ps1'
  @'
param([string]$Repository, [string]$Fixture)
$ErrorActionPreference = 'Stop'
$env:PSModulePath = "$PSHOME\Modules;${env:ProgramFiles}\WindowsPowerShell\Modules"
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  throw 'standard-user coverage must not run with administrator privileges'
}
Write-Output 'Running installer ACL and service/browser checks as a standard Windows user.'
Set-Location $Repository
$env:RUNNER_TEMP = $Fixture
$env:TEMP = $Fixture; $env:TMP = $Fixture
# The noninteractive CI account has no Explorer-initialized special folders.
# Keep its application data inside the fixture owned by this real standard user.
$env:LOCALAPPDATA = Join-Path $Fixture 'LocalAppData'
New-Item -ItemType Directory -Force $env:LOCALAPPDATA | Out-Null
$env:PATH = "$Fixture;$env:PATH"
. ./public/install.ps1 -ValidateFunctionsOnly
Protect-PrivateDirectories @($Root, $Bin, $Data, $Components, $State)
$privateAcl = Get-Acl -LiteralPath $Root
if (!$privateAcl.AreAccessRulesProtected) { throw 'private directory still inherits permissions' }
if ($privateAcl.Access | Where-Object { $_.IdentityReference.Value -in 'Everyone','BUILTIN\Users','NT AUTHORITY\Authenticated Users' }) {
  throw 'private directory grants broad access'
}
& ./scripts/windows-browser-e2e.ps1
if ($LASTEXITCODE) { exit $LASTEXITCODE }
'@ | Set-Content -LiteralPath $worker -Encoding UTF8
  $credential = [Management.Automation.PSCredential]::new("$env:COMPUTERNAME\$name", $secure)
  $stdout = Join-Path $fixture 'stdout.log'; $stderr = Join-Path $fixture 'stderr.log'
  $child = Start-Process -FilePath "$env:SystemRoot\System32\WindowsPowerShell\v1.0\powershell.exe" `
    -Credential $credential -LoadUserProfile -WorkingDirectory $fixture -PassThru `
    -ArgumentList @('-NoLogo','-NoProfile','-NonInteractive','-ExecutionPolicy','Bypass','-File',"`"$worker`"",'-Repository',"`"$repository`"",'-Fixture',"`"$fixture`"") `
    -RedirectStandardOutput $stdout -RedirectStandardError $stderr
  if (!$child.WaitForExit(300000)) { throw 'standard-user browser checks exceeded five minutes' }
  Get-Content -LiteralPath $stdout
  Get-Content -LiteralPath $stderr
  if ($child.ExitCode -ne 0) { throw "standard-user checks failed ($($child.ExitCode))" }
} finally {
  if ($child -and !$child.HasExited) { & taskkill.exe /PID $child.Id /T /F | Out-Null }
  $task = Get-ScheduledTask -TaskName 'OhRats RC Node' -ErrorAction SilentlyContinue
  if ($task -and $user -and $task.Principal.UserId -in @($user.SID.Value, "$env:COMPUTERNAME\$name", $name)) {
    Stop-ScheduledTask -InputObject $task -ErrorAction SilentlyContinue
    Unregister-ScheduledTask -InputObject $task -Confirm:$false
  }
  Remove-LocalUser -Name $name -ErrorAction SilentlyContinue
  if (Test-Path $fixture) { Remove-Item -LiteralPath $fixture -Recurse -Force }
}
