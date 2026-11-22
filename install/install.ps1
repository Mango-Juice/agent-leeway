# agent-leeway installer for Windows (PowerShell 5.1 or later).
#   irm https://raw.githubusercontent.com/Mango-Juice/agent-leeway/main/install/install.ps1 | iex
# Downloads the release zip for this system from GitHub Releases, checks it against checksums.txt, puts
# agent-leeway.exe and its license texts in %LOCALAPPDATA%\Programs\agent-leeway and adds that folder to the USER Path only.
# No administrator rights; the machine Path is never changed. Then runs `agent-leeway install-skills --auto`, which
# adds the skill for Claude Code and Codex if their folders exist (a SKILL.md it did not write is backed up first).
# Optional: $env:LEEWAY_VERSION = 'v0.1.0' (default: latest), $env:LEEWAY_RELEASE_BASE = '<url>',
# $env:LEEWAY_NO_SKILLS = '1' (install the program only).
# Everything runs inside a script block, so nothing (variables, $ErrorActionPreference) is left in the caller's
# session when it is run as `irm ... | iex`.
& {
  $ErrorActionPreference = 'Stop'
  $Owner = 'Mango-Juice'   # keep in sync with internal/meta/meta.go
  $Repo = 'agent-leeway'
  $Name = 'agent-leeway'

  $Version = if ($env:LEEWAY_VERSION) { $env:LEEWAY_VERSION } else { 'latest' }
  if ($env:LEEWAY_RELEASE_BASE) { $Base = $env:LEEWAY_RELEASE_BASE }
  elseif ($Version -eq 'latest') { $Base = "https://github.com/$Owner/$Repo/releases/latest/download" }
  else { $Base = "https://github.com/$Owner/$Repo/releases/download/$Version" }

  switch ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { $Arch = 'amd64' }
    'ARM64' { $Arch = 'arm64' }
    default { throw "$Name install: unsupported processor $($env:PROCESSOR_ARCHITECTURE)" }
  }
  $Archive = "${Name}_windows_${Arch}.zip"
  $Dest = if ($env:LEEWAY_INSTALL_DIR) { $env:LEEWAY_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "Programs\$Name" }
  $Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("$Name-" + [System.Guid]::NewGuid().ToString('N'))
  New-Item -ItemType Directory -Path $Tmp | Out-Null
  try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    Write-Host "Downloading $Archive ..."
    Invoke-WebRequest -UseBasicParsing -Uri "$Base/$Archive" -OutFile (Join-Path $Tmp $Archive)
    Invoke-WebRequest -UseBasicParsing -Uri "$Base/checksums.txt" -OutFile (Join-Path $Tmp 'checksums.txt')
    $Want = $null
    foreach ($line in Get-Content (Join-Path $Tmp 'checksums.txt')) {
      $parts = $line -split '\s+'
      if ($parts.Count -ge 2 -and ($parts[1] -eq $Archive -or $parts[1] -eq "*$Archive")) { $Want = $parts[0].ToLower() }
    }
    if (-not $Want) { throw "$Name install: $Archive is not listed in checksums.txt" }
    $Got = (Get-FileHash -Algorithm SHA256 -Path (Join-Path $Tmp $Archive)).Hash.ToLower()
    if ($Want -ne $Got) { throw "$Name install: checksum mismatch for $Archive (expected $Want, got $Got); nothing was installed" }
    Expand-Archive -Path (Join-Path $Tmp $Archive) -DestinationPath (Join-Path $Tmp 'x') -Force
    $Exe = Join-Path $Tmp "x\$Name.exe"
    if (-not (Test-Path $Exe)) { throw "$Name install: $Archive does not contain $Name.exe" }
    New-Item -ItemType Directory -Force -Path $Dest | Out-Null
    Copy-Item -Force $Exe (Join-Path $Dest "$Name.exe")
    Write-Host "Installed $(Join-Path $Dest "$Name.exe")"
    # license texts that come with the program (its own and the third-party ones compiled into it)
    foreach ($f in @('LICENSE', 'NOTICE.md', 'THIRD_PARTY_LICENSES.txt')) {
      $src = Join-Path $Tmp "x\$f"
      if (Test-Path $src) { Copy-Item -Force $src (Join-Path $Dest $f) }
    }
    # Read the user Path unexpanded and write it back as REG_EXPAND_SZ, so entries such as %USERPROFILE%\bin
    # keep following their variables (GetEnvironmentVariable would expand them and store literal text).
    $Key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $true)
    try {
      $UserPath = [string]$Key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
      $Entries = @($UserPath -split ';' | Where-Object { $_ -ne '' })
      $Expanded = $Entries | ForEach-Object { [Environment]::ExpandEnvironmentVariables($_).TrimEnd('\') }
      if (-not ($Expanded | Where-Object { $_ -ieq $Dest.TrimEnd('\') })) {
        $Key.SetValue('Path', (($Entries + $Dest) -join ';'), [Microsoft.Win32.RegistryValueKind]::ExpandString)
        # announce the change (WM_SETTINGCHANGE) so new terminals see it: SetEnvironmentVariable broadcasts
        [Environment]::SetEnvironmentVariable('AGENT_LEEWAY_PATH_REFRESH', '1', 'User')
        [Environment]::SetEnvironmentVariable('AGENT_LEEWAY_PATH_REFRESH', $null, 'User')
        Write-Host "Added $Dest to your user Path. Open a new terminal to use $Name."
      }
    } finally {
      $Key.Close()
    }
    # The skill for Claude Code and Codex. Never fails the install: the program is in place either way.
    # install-skills --auto: 0 = written or up to date, 3 = no agent folder found (it says so), else an error.
    $Installed = Join-Path $Dest "$Name.exe"
    $Skills = $false
    if ($env:LEEWAY_NO_SKILLS -and $env:LEEWAY_NO_SKILLS -ne '0') {
      Write-Host 'Skills: not added (LEEWAY_NO_SKILLS is set).'
    } else {
      $Rc = 1
      $Eap = $ErrorActionPreference
      try {
        # stderr of a native program must not stop the script here (Windows PowerShell 5.1 turns it into errors)
        $ErrorActionPreference = 'Continue'
        $Help = (& $Installed install-skills -h 2>&1 | ForEach-Object { "$_" }) -join "`n"
        if ($Help -match '-auto') {
          & $Installed install-skills --auto | ForEach-Object { Write-Host $_ }; $Rc = $LASTEXITCODE
        } else {
          # a release from before --auto (v0.1.0): plain install-skills, whose "none found" message means 3 here
          $Out = @(& $Installed install-skills 2>&1 | ForEach-Object { "$_" }); $Rc = $LASTEXITCODE
          if (($Out -join "`n") -like '*Neither Claude Code nor Codex was found*') {
            $Rc = 3
            Write-Host 'Skills: not added. No Claude Code or Codex found on this computer (no .claude or .codex folder in your user folder).'
          } else {
            $Out | Where-Object { $_ -notlike 'Ask your agent for a*' } | ForEach-Object { Write-Host $_ }
          }
        }
      } catch { Write-Host "$_" } finally { $ErrorActionPreference = $Eap }
      # the install itself succeeded: do not leave install-skills' exit code (3 = no agent) in the caller's session
      $global:LASTEXITCODE = 0
      if ($Rc -eq 0) { $Skills = $true }
      elseif ($Rc -ne 3) { Write-Host "Skills: not added (see the message above). The program is installed; run ``$Name install-skills`` to try again." }
    }
    if ($Skills) {
      Write-Host "Next: ask Claude Code or Codex for a `"leeway report`". In a terminal instead: $Name report"
    } else {
      Write-Host "Next: run ``$Name install-skills`` once (after installing Claude Code or Codex, if neither is here), then ask Claude Code or Codex for a `"leeway report`". In a terminal instead: $Name report"
    }
  } finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
  }
}
