# agent-leeway installer for Windows (PowerShell 5.1 or later).
#   irm https://raw.githubusercontent.com/Mango-Juice/agent-leeway/main/install/install.ps1 | iex
# Downloads the release zip for this system from GitHub Releases, checks it against checksums.txt, puts
# agent-leeway.exe and its license texts in %LOCALAPPDATA%\Programs\agent-leeway and adds that folder to the USER Path only.
# No administrator rights; the machine Path is never changed.
# Optional: $env:LEEWAY_VERSION = 'v0.1.0' (default: latest), $env:LEEWAY_RELEASE_BASE = '<url>'.
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
    Write-Host "Next: $Name install-skills   (adds the skill to Claude Code and Codex), then ask your agent for a leeway report."
  } finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
  }
}
