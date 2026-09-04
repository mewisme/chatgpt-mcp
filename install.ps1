# chatgpt-mcp bootstrap installer for Windows (PowerShell).
#
# irm https://get.mewis.me/chatgpt-mcp.ps1 | iex
#
# Environment:
#   CHATGPT_MCP_VERSION      release tag (default: latest)
#   CHATGPT_MCP_INSTALL_DIR  install location (default: %LOCALAPPDATA%\chatgpt-mcp)

param(
  [switch]$Uninstall,
  [switch]$NoAlias
)

$ErrorActionPreference = 'Stop'
$repo = 'mewisme/chatgpt-mcp'
$defaultInstall = Join-Path $env:LOCALAPPDATA 'chatgpt-mcp'
$installDir = if ($env:CHATGPT_MCP_INSTALL_DIR) { $env:CHATGPT_MCP_INSTALL_DIR } else { $defaultInstall }
$current = Join-Path $installDir 'current'

if ($Uninstall) {
  if (Test-Path $installDir) { Remove-Item -Recurse -Force $installDir }
  if ($installDir -eq $defaultInstall) {
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath) {
      $nextPath = (($userPath -split ';') | Where-Object { $_ -and $_ -ne $current }) -join ';'
      [Environment]::SetEnvironmentVariable('Path', $nextPath, 'User')
    }
  }
  Write-Host "chatgpt-mcp uninstalled from $installDir"
  return
}

$arch = switch ([System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture) {
  'Arm64' { 'arm64' }
  'X64' { 'amd64' }
  default { throw "chatgpt-mcp: unsupported architecture '$_'." }
}

$version = $env:CHATGPT_MCP_VERSION
if (-not $version) {
  $version = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
}
if (-not $version) { throw 'chatgpt-mcp: could not resolve latest version; set CHATGPT_MCP_VERSION.' }
if ($version -notmatch '^v') { $version = "v$version" }
$ver = $version.TrimStart('v')
$asset = "chatgpt-mcp_${ver}_windows_${arch}.zip"
$url = "https://github.com/$repo/releases/download/$version/$asset"
$checksumsUrl = "https://github.com/$repo/releases/download/$version/checksums.txt"
Write-Host "Installing chatgpt-mcp $version (windows/$arch)..."

$tmp = Join-Path $env:TEMP ("chatgpt-mcp-" + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
try {
  $zip = Join-Path $tmp $asset
  $checksums = Join-Path $tmp 'checksums.txt'
  Invoke-WebRequest -Uri $url -OutFile $zip
  Invoke-WebRequest -Uri $checksumsUrl -OutFile $checksums
  $expected = Get-Content $checksums | ForEach-Object {
    if ($_ -match '^([0-9a-fA-F]{64})\s+(.+)$' -and $Matches[2] -eq $asset) { $Matches[1].ToLowerInvariant() }
  } | Select-Object -First 1
  if (-not $expected) { throw "chatgpt-mcp: checksum missing for $asset" }
  $actual = (Get-FileHash -Algorithm SHA256 -Path $zip).Hash.ToLowerInvariant()
  if ($actual -ne $expected) { throw "chatgpt-mcp: checksum verification failed for $asset" }

  $extract = Join-Path $tmp 'extract'
  Expand-Archive -Path $zip -DestinationPath $extract -Force
  $exe = Join-Path $extract 'chatgpt-mcp.exe'
  if (-not (Test-Path $exe)) { throw 'chatgpt-mcp: chatgpt-mcp.exe missing from archive.' }

  $installArgs = @('install')
  if ($NoAlias) { $installArgs += '--no-alias' }
  & $exe @installArgs
  if ($LASTEXITCODE -ne 0) { throw "chatgpt-mcp: self-install failed with exit code $LASTEXITCODE" }
} finally {
  if (Test-Path $tmp) { Remove-Item -Recurse -Force $tmp }
}

if ($installDir -eq $defaultInstall) {
  $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
  $entries = if ($userPath) { $userPath -split ';' } else { @() }
  if ($entries -notcontains $current) {
    $nextPath = if ($userPath) { "$current;$userPath" } else { $current }
    [Environment]::SetEnvironmentVariable('Path', $nextPath, 'User')
    $env:Path = "$current;$env:Path"
    Write-Host "Added $current to your PATH (restart your terminal if needed)."
  }
} elseif (($env:Path -split ';') -notcontains $current) {
  Write-Host ''
  Write-Host "$current is not on your PATH. Add it to use chatgpt-mcp from any terminal."
}

Write-Host ''
if ($NoAlias) {
  Write-Host 'Done. Run: chatgpt-mcp --help'
} else {
  Write-Host 'Done. Run: chatgpt-mcp --help or cgm --help'
}
