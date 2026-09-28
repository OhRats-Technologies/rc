$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Console]::InputEncoding = [Text.UTF8Encoding]::new($false)
$spec = [Console]::In.ReadToEnd() | ConvertFrom-Json
$sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$scheduler = New-Object -ComObject 'Schedule.Service'
$scheduler.Connect()
$task = $scheduler.NewTask(0)
$task.RegistrationInfo.Description = 'RC per-user background Node'
$task.Principal.UserId = $sid
$task.Principal.LogonType = 3 # TASK_LOGON_INTERACTIVE_TOKEN, no password or elevation.
$task.Principal.RunLevel = 0 # TASK_RUNLEVEL_LUA.
$trigger = $task.Triggers.Create(9) # TASK_TRIGGER_LOGON.
$trigger.UserId = $sid # An all-users logon trigger requires administrator privileges.
$action = $task.Actions.Create(0)
$action.Path = $spec.executable
$action.Arguments = $spec.arguments
$action.WorkingDirectory = Split-Path -Parent $spec.executable
$task.Settings.Enabled = $true
$task.Settings.ExecutionTimeLimit = 'PT0S'
$task.Settings.DisallowStartIfOnBatteries = $false
$task.Settings.StopIfGoingOnBatteries = $false
$task.Settings.MultipleInstances = 2 # TASK_INSTANCES_IGNORE_NEW.
$task.Settings.RestartCount = 10
$task.Settings.RestartInterval = 'PT1M'
$task.Settings.StartWhenAvailable = $true
# Explicit ownership also makes a task installed from an elevated shell
# maintainable from the same user's normal, limited shell.
$security = "O:${sid}D:P(A;;FA;;;${sid})(A;;FA;;;SY)(A;;FA;;;BA)"
try {
  $registered = $scheduler.GetFolder('\').RegisterTaskDefinition('OhRats RC Node', $task, 6, $sid, $null, 3, $security)
} catch {
  $cause = $_.Exception
  while ($cause.InnerException) { $cause = $cause.InnerException }
  if ($cause.HResult -eq -2147024891) {
    throw 'RC task access denied. An older administrator-owned task may need a one-time repair: run scripts/repair-windows-service.ps1 from an elevated PowerShell as the enrolled user, then rerun the installer in normal PowerShell. Enrollment is preserved.'
  }
  throw
}
[void]$registered.Run($null)
Write-Output 'RC background service registered for the current Windows user.'
