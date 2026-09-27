$ErrorActionPreference = 'Stop'
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
$registered = $scheduler.GetFolder('\').RegisterTaskDefinition('OhRats RC Node', $task, 6, $sid, $null, 3)
[void]$registered.Run($null)
Write-Output 'RC background service registered for the current Windows user.'
