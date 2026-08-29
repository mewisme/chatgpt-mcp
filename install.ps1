# chatgpt-mcp installer for Windows (PowerShell).
#
# irm https://raw.githubusercontent.com/mewisme/chatgpt-mcp/main/install.ps1 | iex
#
# Environment:
#   CHATGPT_MCP_VERSION      release tag (default: latest)
#   CHATGPT_MCP_INSTALL_DIR  install location (default: %LOCALAPPDATA%\chatgpt-mcp)

param([switch]$Uninstall)

$ErrorActionPreference = 'Stop'
$repo = 'mewisme/chatgpt-mcp'
$defaultInstall = Join-Path $env:LOCALAPPDATA 'chatgpt-mcp'
$installDir = if ($env:CHATGPT_MCP_INSTALL_DIR) { $env:CHATGPT_MCP_INSTALL_DIR } else { $defaultInstall }
$dest = Join-Path $installDir 'current'

if ($Uninstall) {
  if (Test-Path $installDir) { Remove-Item -Recurse -Force $installDir }
  if ($installDir -eq $defaultInstall) {
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ($userPath) {
      $nextPath = (($userPath -split ';') | Where-Object { $_ -and $_ -ne $dest }) -join ';'
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

$url = "https://github.com/$repo/releases/download/$version/chatgpt-mcp_${ver}_windows_${arch}.zip"
Write-Host "Installing chatgpt-mcp $version (windows/$arch)..."

$tmp = Join-Path $env:TEMP ("chatgpt-mcp-" + [guid]::NewGuid().ToString())
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
try {
  $zip = Join-Path $tmp 'chatgpt-mcp.zip'
  Invoke-WebRequest -Uri $url -OutFile $zip

  if (Test-Path $dest) { Remove-Item -Recurse -Force $dest }
  New-Item -ItemType Directory -Force -Path $dest | Out-Null
  Expand-Archive -Path $zip -DestinationPath $dest -Force
} finally {
  if (Test-Path $tmp) { Remove-Item -Recurse -Force $tmp }
}

$exe = Join-Path $dest 'chatgpt-mcp.exe'
if (-not (Test-Path $exe)) { throw 'chatgpt-mcp: chatgpt-mcp.exe missing from archive.' }

if ($installDir -eq $defaultInstall) {
  $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
  $entries = if ($userPath) { $userPath -split ';' } else { @() }
  if ($entries -notcontains $dest) {
    $nextPath = if ($userPath) { "$dest;$userPath" } else { $dest }
    [Environment]::SetEnvironmentVariable('Path', $nextPath, 'User')
    $env:Path = "$dest;$env:Path"
    Write-Host "Added $dest to your PATH (restart your terminal if needed)."
  }
}

Write-Host "Installed to $dest"
Write-Host 'Run: chatgpt-mcp --help'
