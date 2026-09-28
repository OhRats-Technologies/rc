# Run once as the enrolled user in an elevated PowerShell if an older task
# grants that user's limited token only read access. Does not start the Node.
$ErrorActionPreference = 'Stop'
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = [Security.Principal.WindowsPrincipal]::new($identity)
if (!$principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
  throw 'Open PowerShell as administrator using the enrolled Windows account, then rerun this script.'
}
$scheduler = New-Object -ComObject 'Schedule.Service'
$scheduler.Connect()
$task = $scheduler.GetFolder('\').GetTask('OhRats RC Node')
$owner = $task.Definition.Principal.UserId
$sid = $identity.User.Value
if (!$owner.StartsWith('S-1-')) {
  $owner = [Security.Principal.NTAccount]::new($owner).Translate([Security.Principal.SecurityIdentifier]).Value
}
if ($owner -ne $sid) { throw 'Refusing to change an RC task belonging to another Windows user.' }
$task.SetSecurityDescriptor("O:${sid}D:P(A;;FA;;;${sid})(A;;FA;;;SY)(A;;FA;;;BA)", 0)
Write-Output 'RC task permissions repaired. Rerun the installer in normal PowerShell.'
