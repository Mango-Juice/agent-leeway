# CI test of install/install.ps1 on Windows (run by .github/workflows/ci.yml).
# Builds agent-leeway.exe, makes a local "release" (zip + checksums.txt) served over http://127.0.0.1, then
# installs it the way users do (irm <url> | iex) and checks:
#   1. the program is installed and runs; license files come with it
#   2. the install folder is added to the USER Path only; the machine Path is unchanged; a second install
#      does not add it twice
#   3. a checksum mismatch fails and installs nothing
#   4. install-skills / uninstall-skills write and remove only their own files (temporary home)
#   4b. the installer adds the skill itself (both agents, re-run, LEEWAY_NO_SKILLS, no agent present)
#   4c. the same with a release from before install-skills --auto (a v0.1.0 stand-in)
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
# a stand-in for the v0.1.0 release: install-skills without --auto (internal/install/testdata/v010stub)
$OldBuild = Join-Path $Work 'build old'
New-Item -ItemType Directory -Path $OldBuild | Out-Null
Push-Location $Repo
& go build -trimpath -o (Join-Path $OldBuild "$Name.exe") ./internal/install/testdata/v010stub
if ($LASTEXITCODE -ne 0) { throw 'go build of the v0.1.0 stand-in failed' }
Pop-Location
Get-ChildItem -Path $Build -Exclude "$Name.exe" | Copy-Item -Destination $OldBuild
$Serve = Join-Path $Work 'serve'
$d = Join-Path $Serve 'old'
New-Item -ItemType Directory -Force -Path $d | Out-Null
Compress-Archive -Path (Join-Path $OldBuild '*') -DestinationPath (Join-Path $d $Archive)
$h = (Get-FileHash -Algorithm SHA256 (Join-Path $d $Archive)).Hash.ToLower()
Set-Content -Path (Join-Path $d 'checksums.txt') -Value "$h  $Archive" -Encoding ascii
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
  $env:LEEWAY_NO_SKILLS = '1'   # sections 1-3 leave the runner's own profile alone; 4b tests the skill step
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

  # --- 4b: the installer's skill step, in temporary homes -----------------------------------------------------
  $env:LEEWAY_RELEASE_BASE = "$Url/good"; $env:LEEWAY_INSTALL_DIR = $GoodDest
  $saved = @{ USERPROFILE = $env:USERPROFILE; HOME = $env:HOME; CLAUDE_CONFIG_DIR = $env:CLAUDE_CONFIG_DIR; CODEX_HOME = $env:CODEX_HOME; LEEWAY_NO_SKILLS = $env:LEEWAY_NO_SKILLS }
  function InstallInto($h, $optOut) {
    $env:USERPROFILE = $h; $env:HOME = $h
    Remove-Item env:CLAUDE_CONFIG_DIR, env:CODEX_HOME -ErrorAction SilentlyContinue
    if ($optOut) { $env:LEEWAY_NO_SKILLS = '1' } else { Remove-Item env:LEEWAY_NO_SKILLS -ErrorAction SilentlyContinue }
    # join the records as they are: Out-String would wrap long lines at the host width
    $o = (& { irm "$Url/install.ps1" | iex } 6>&1 | ForEach-Object { "$_" }) -join "`n"
    Write-Host $o
    return $o
  }
  try {
    $H1 = Join-Path $Work 'home both'
    New-Item -ItemType Directory -Force -Path (Join-Path $H1 '.claude'), (Join-Path $H1 '.codex') | Out-Null
    $own = Join-Path $H1 '.agents\skills\agent-leeway\SKILL.md'
    New-Item -ItemType Directory -Force -Path (Split-Path $own) | Out-Null
    Set-Content -Path $own -Value 'my own skill'
    $o = InstallInto $H1 $false
    $c1 = Join-Path $H1 '.claude\skills\agent-leeway\SKILL.md'
    Check ((Test-Path $c1) -and ((Get-Content -Raw $own) -like '*--tool codex*')) 'installer added both skills'
    Check ($o -like '*Claude Code: added the skill:*(undo: agent-leeway uninstall-skills --claude)*') 'installer says what it wrote and how to undo it'
    Check ($o -like '*Next: ask Claude Code or Codex for a "leeway report"*') 'closing line is the next action'
    Check (@(Get-ChildItem -Path (Split-Path $own) -Filter 'SKILL.md.bak-*').Count -eq 1) "the user's own skill was backed up"
    $o = InstallInto $H1 $false
    Check (([regex]::Matches($o, 'the skill is already up to date')).Count -eq 2) 're-run leaves the skills as they are'
    Check (@(Get-ChildItem -Path (Split-Path $own) -Filter 'SKILL.md.bak-*').Count -eq 1) 're-run makes no second backup'
    $H2 = Join-Path $Work 'home optout'
    New-Item -ItemType Directory -Force -Path (Join-Path $H2 '.claude') | Out-Null
    $o = InstallInto $H2 $true
    Check (($o -like '*Skills: not added (LEEWAY_NO_SKILLS is set).*') -and -not (Test-Path (Join-Path $H2 '.claude\skills'))) 'LEEWAY_NO_SKILLS skips the skill'
    $H3 = Join-Path $Work 'home none'
    New-Item -ItemType Directory -Force -Path $H3 | Out-Null
    $o = InstallInto $H3 $false
    Check (($o -like '*Skills: not added. No Claude Code or Codex found*') -and ($o -notlike '*see the message above*') -and (Test-Path $LeewayExe)) 'no agent: one line, install still done'
    Check ($LASTEXITCODE -eq 0) 'the installer leaves $LASTEXITCODE at 0'

    # --- 4c: a release from before --auto ---------------------------------------------------------------------
    $env:LEEWAY_RELEASE_BASE = "$Url/old"
    $H4 = Join-Path $Work 'home old both'
    New-Item -ItemType Directory -Force -Path (Join-Path $H4 '.claude'), (Join-Path $H4 '.codex') | Out-Null
    $o = InstallInto $H4 $false
    Check ((Test-Path (Join-Path $H4 '.claude\skills\agent-leeway\SKILL.md')) -and (Test-Path (Join-Path $H4 '.agents\skills\agent-leeway\SKILL.md'))) 'older release: installer added both skills'
    Check (($o -like '*Next: ask Claude Code or Codex for a "leeway report"*') -and ($o -notlike '*flag provided but not defined*') -and ($o -notlike '*see the message above*')) 'older release: no flag error, next action shown'
    $H5 = Join-Path $Work 'home old none'
    New-Item -ItemType Directory -Force -Path $H5 | Out-Null
    $o = InstallInto $H5 $false
    Check (($o -like '*Skills: not added. No Claude Code or Codex found*') -and ($o -notlike '*Neither Claude Code*') -and ($o -notlike '*see the message above*')) 'older release, no agent: one line'
    Check ($LASTEXITCODE -eq 0) 'older release: $LASTEXITCODE left at 0'
    $env:LEEWAY_RELEASE_BASE = "$Url/good"
  } finally {
    foreach ($k in $saved.Keys) { if ($null -eq $saved[$k]) { Remove-Item "env:$k" -ErrorAction SilentlyContinue } else { Set-Item -Path "env:$k" -Value $saved[$k] } }
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
exit 0   # the runner's wrapper exits with $LASTEXITCODE otherwise
