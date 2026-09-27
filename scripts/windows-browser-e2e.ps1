$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$chrome = @(
  (Join-Path $env:ProgramFiles 'Google\Chrome\Application\chrome.exe'),
  (Join-Path ${env:ProgramFiles(x86)} 'Google\Chrome\Application\chrome.exe')
) | Where-Object { Test-Path $_ } | Select-Object -First 1
if (!$chrome) { throw 'Chrome is required for native Windows browser conformance' }
$profile = Join-Path $env:RUNNER_TEMP 'rc-chrome-e2e'
$start = [Diagnostics.ProcessStartInfo]::new()
$start.FileName = $chrome
$start.UseShellExecute = $false
foreach ($argument in @(
  '--headless=new', '--no-sandbox', '--no-first-run', '--no-default-browser-check',
  '--disable-gpu', '--disable-background-networking',
  '--disable-features=WebRtcHideLocalIpsWithMdns', '--remote-allow-origins=*',
  '--remote-debugging-address=127.0.0.1', '--remote-debugging-port=9223',
  "--user-data-dir=$profile", 'about:blank'
)) { $start.ArgumentList.Add($argument) }
$state = Join-Path $env:RUNNER_TEMP ('rc-browser-e2e-' + [guid]::NewGuid())
New-Item -ItemType Directory $state | Out-Null
$env:RC_E2E_DIRECTORY = $state
$env:RC_E2E_KEEP = '1'
$browser = [Diagnostics.Process]::Start($start)
try {
  $ready = $false
  for ($attempt = 0; $attempt -lt 100; $attempt++) {
    try {
      Invoke-RestMethod 'http://127.0.0.1:9223/json/version' | Out-Null
      $ready = $true
      break
    } catch { Start-Sleep -Milliseconds 200 }
  }
  if (!$ready) { throw 'Chrome DevTools did not become ready' }
  $env:RC_CDP_URL = 'http://127.0.0.1:9223'
  bun run smoke:browser-e2e
  if ($LASTEXITCODE) { throw 'native Windows browser conformance failed' }
} finally {
  if (!$browser.HasExited) { $browser.Kill($true); $browser.WaitForExit() }
  $browser.Dispose()
  # Bun can retain directory handles until exit; cleanup belongs to the parent.
  for ($attempt = 0; $attempt -lt 20; $attempt++) {
    try { Remove-Item -LiteralPath $state -Recurse -Force; break }
    catch { if ($attempt -eq 19) { throw }; Start-Sleep -Milliseconds 250 }
  }
  Remove-Item Env:RC_E2E_DIRECTORY,Env:RC_E2E_KEEP
}
