# CI test of install/install.ps1 on Windows (run by .github/workflows/ci.yml).
# Builds agent-leeway.exe, makes a local "release" (zip + checksums.txt) served over http://127.0.0.1, then
# installs it the way users do (irm <url> | iex) and checks:
#   1. the program is installed and runs; license files come with it
#   2. the install folder is added to the USER Path only; the machine Path is unchanged; a second install
#      does not add it twice
#   3. a checksum mismatch fails and installs nothing
#   4. install-skills / uninstall-skills write and remove only their own files (temporary home)
#   5. uninstall as documented (delete the folder, remove it from the user Path) leaves nothing behind
# install.ps1 runs in this script's scope (irm | iex), so its variables ($Exe, $Dest, $Tmp, ...) land here too;
# this script uses other names ($LeewayExe, $GoodDest, $BadInstallDir) and only shares ones with equal values ($Name, $Arch, $Archive).
# Usage: powershell -NoProfile -File .github/scripts/test-install-ps1.ps1   (also works under pwsh)
$ErrorActionPreference = 'Stop'
$Repo = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
$Name = 'agent-leeway'
$Arch = switch ($env:PROCESSOR_ARCHITECTURE) { 'AMD64' { 'amd64' } 'ARM64' { 'arm64' } default { throw "unsupported $($env:PROCESSOR_ARCHITECTURE)" } }
$Archive = "${Name}_windows_${Arch}.zip"
$Work = Join-Path ([IO.Path]::GetTempPath()) ("leeway ps1 test " + [Guid]::NewGuid().ToString('N'))   # space on purpose
New-Item -ItemType Directory -Path $Work | Out-Null
$fail = 0
function Check($cond, $msg) { if ($cond) { Write-Host "ok   $msg" } else { Write-Host "FAIL $msg"; $script:fail++ } }

# --- build and make two releases: good and corrupt -------------------------------------------------------------
$Build = Join-Path $Work 'build'
New-Item -ItemType Directory -Path $Build | Out-Null
Push-Location $Repo
& go build -trimpath -o (Join-Path $Build "$Name.exe") ./cmd/agent-leeway
if ($LASTEXITCODE -ne 0) { throw 'go build failed' }
Pop-Location
foreach ($f in @('LICENSE', 'NOTICE.md', 'THIRD_PARTY_LICENSES.txt')) {
  $src = Join-Path $Repo $f
  if (Test-Path $src) { Copy-Item $src $Build } else { Set-Content -Path (Join-Path $Build $f) -Value "text of $f" }
}
$Serve = Join-Path $Work 'serve'
foreach ($kind in @('good', 'bad')) {
  $d = Join-Path $Serve $kind
  New-Item -ItemType Directory -Force -Path $d | Out-Null
  Compress-Archive -Path (Join-Path $Build '*') -DestinationPath (Join-Path $d $Archive)
  $h = (Get-FileHash -Algorithm SHA256 (Join-Path $d $Archive)).Hash.ToLower()
  if ($kind -eq 'bad') { $h = '0' * 64 }
  Set-Content -Path (Join-Path $d 'checksums.txt') -Value "$h  $Archive" -Encoding ascii
}
Copy-Item (Join-Path $Repo 'install\install.ps1') (Join-Path $Serve 'install.ps1')

# --- a tiny static file server in a background job --------------------------------------------------------------
$Port = Get-Random -Minimum 20000 -Maximum 40000
$Server = Start-Job -ArgumentList $Serve, $Port -ScriptBlock {
  param($root, $port)
  $l = New-Object System.Net.HttpListener
  $l.Prefixes.Add("http://127.0.0.1:$port/")
  $l.Start()
  while ($l.IsListening) {
    $c = $l.GetContext()
    $p = Join-Path $root ($c.Request.Url.AbsolutePath.TrimStart('/') -replace '/', '\')
    if (Test-Path $p -PathType Leaf) {
      $b = [IO.File]::ReadAllBytes($p); $c.Response.ContentLength64 = $b.Length
      $c.Response.OutputStream.Write($b, 0, $b.Length)
    } else { $c.Response.StatusCode = 404 }
    $c.Response.Close()
  }
}
$Url = "http://127.0.0.1:$Port"
for ($i = 0; $i -lt 50; $i++) {
  try { Invoke-WebRequest -UseBasicParsing "$Url/install.ps1" | Out-Null; break } catch { Start-Sleep -Milliseconds 200 }
}

$UserPathBefore = [Environment]::GetEnvironmentVariable('Path', 'User')
$MachinePathBefore = [Environment]::GetEnvironmentVariable('Path', 'Machine')
try {
  # --- 1, 2: install as users do ---------------------------------------------------------------------------------
  $GoodDest = Join-Path $Work 'install dir'
  $env:LEEWAY_RELEASE_BASE = "$Url/good"; $env:LEEWAY_INSTALL_DIR = $GoodDest
  irm "$Url/install.ps1" | iex
  $LeewayExe = Join-Path $GoodDest "$Name.exe"
  Check (Test-Path $LeewayExe) 'program installed'
  $ver = & $LeewayExe --version
  Check ($LASTEXITCODE -eq 0 -and "$ver" -like "$Name *") "installed program runs ($ver)"
  foreach ($f in @('LICENSE', 'NOTICE.md', 'THIRD_PARTY_LICENSES.txt')) { Check (Test-Path (Join-Path $GoodDest $f)) "$f installed" }
  $up = [Environment]::GetEnvironmentVariable('Path', 'User')
  $hits = @($up -split ';' | Where-Object { $_.TrimEnd('\') -ieq $GoodDest.TrimEnd('\') })
  Check ($hits.Count -eq 1) 'install folder added to the user Path once'
  Check ([Environment]::GetEnvironmentVariable('Path', 'Machine') -eq $MachinePathBefore) 'machine Path unchanged'
  irm "$Url/install.ps1" | iex   # reinstall
  $up = [Environment]::GetEnvironmentVariable('Path', 'User')
  $hits = @($up -split ';' | Where-Object { $_.TrimEnd('\') -ieq $GoodDest.TrimEnd('\') })
  Check ($hits.Count -eq 1) 'reinstall does not add the folder twice'

  # --- 3: checksum mismatch --------------------------------------------------------------------------------------
  $BadInstallDir = Join-Path $Work 'bad dir'
  $env:LEEWAY_RELEASE_BASE = "$Url/bad"; $env:LEEWAY_INSTALL_DIR = $BadInstallDir
  $msg = ''
  try { irm "$Url/install.ps1" | iex } catch { $msg = "$_" }
  Check ($msg -like '*checksum mismatch*') 'checksum mismatch is reported'
  Check (-not (Test-Path (Join-Path $BadInstallDir "$Name.exe"))) 'nothing installed on a checksum mismatch'

  # --- 4: skills in a temporary home -----------------------------------------------------------------------------
  $FakeHome = Join-Path $Work 'home'
  New-Item -ItemType Directory -Path $FakeHome | Out-Null
  $saved = @{ USERPROFILE = $env:USERPROFILE; HOME = $env:HOME; CLAUDE_CONFIG_DIR = $env:CLAUDE_CONFIG_DIR }
  $env:USERPROFILE = $FakeHome; $env:HOME = $FakeHome; $env:CLAUDE_CONFIG_DIR = (Join-Path $FakeHome '.claude')
  try {
    & $LeewayExe install-skills --claude --codex | Out-Host
    $claudeSkill = Join-Path $FakeHome '.claude\skills\agent-leeway\SKILL.md'
    $codexSkill = Join-Path $FakeHome '.agents\skills\agent-leeway\SKILL.md'
    Check ((Test-Path $claudeSkill) -and (Test-Path $codexSkill)) 'install-skills wrote both skills'
    & $LeewayExe uninstall-skills --claude --codex | Out-Host
    Check (-not (Test-Path $claudeSkill) -and -not (Test-Path $codexSkill)) 'uninstall-skills removed both skills'
  } finally {
    foreach ($k in $saved.Keys) { Set-Item -Path "env:$k" -Value $saved[$k] -ErrorAction SilentlyContinue }
  }

  # --- 5: uninstall as documented --------------------------------------------------------------------------------
  Remove-Item -Recurse -Force $GoodDest
  $keep = @([Environment]::GetEnvironmentVariable('Path', 'User') -split ';' | Where-Object { $_ -ne '' -and $_.TrimEnd('\') -ine $GoodDest.TrimEnd('\') })
  [Environment]::SetEnvironmentVariable('Path', ($keep -join ';'), 'User')
  Check (-not (Test-Path $GoodDest)) 'install folder removed'
  Check (-not (([Environment]::GetEnvironmentVariable('Path', 'User') -split ';') | Where-Object { $_.TrimEnd('\') -ieq $GoodDest.TrimEnd('\') })) 'install folder no longer on the user Path'
} finally {
  [Environment]::SetEnvironmentVariable('Path', $UserPathBefore, 'User')
  Stop-Job $Server -ErrorAction SilentlyContinue; Remove-Job $Server -Force -ErrorAction SilentlyContinue
  Remove-Item -Recurse -Force $Work -ErrorAction SilentlyContinue
}
if ($fail -gt 0) { Write-Host "$fail check(s) failed"; exit 1 }
Write-Host 'all install.ps1 checks passed'
