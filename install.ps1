# chatgpt-mcp bootstrap installer for Windows (PowerShell).
#
# irm https://get.mewis.me/chatgpt-mcp.ps1 | iex
#
# Environment:
#   CHATGPT_MCP_VERSION      release tag (default: latest)
#   CHATGPT_MCP_INSTALL_DIR  install location (default: %LOCALAPPDATA%\chatgpt-mcp)
#   CHATGPT_MCP_ARCH         architecture override: amd64 or arm64

param(
  [switch]$Uninstall,
  [switch]$NoAlias
)

$ErrorActionPreference = 'Stop'
$repo = 'mewisme/chatgpt-mcp'
$defaultInstall = Join-Path $env:LOCALAPPDATA 'chatgpt-mcp'
$installDir = if ($env:CHATGPT_MCP_INSTALL_DIR) { $env:CHATGPT_MCP_INSTALL_DIR } else { $defaultInstall }
$current = Join-Path $installDir 'current'

function ConvertTo-ChatGPTMCPArchitecture {
  param([AllowNull()][object]$Value)
  if ($null -eq $Value) { return $null }
  $text = ([string]$Value).Trim()
  if (-not $text) { return $null }
  switch -Regex ($text.ToUpperInvariant()) {
    '^(AMD64|X64|X86_64)$' { return 'amd64' }
    '^(ARM64|AARCH64)$' { return 'arm64' }
    'ARM.*64' { return 'arm64' }
    'INTEL64|AMD64' { return 'amd64' }
    default { return $null }
  }
}

function Get-ChatGPTMCPRuntimeArchitecture {
  try {
    $value = [System.Runtime.InteropServices.RuntimeInformation]::OSArchitecture
    if ($null -ne $value) { return $value.ToString() }
  } catch {}
  return $null
}

function Get-ChatGPTMCPOSArchitecture {
  try {
    return (Get-CimInstance -ClassName Win32_OperatingSystem -ErrorAction Stop | Select-Object -First 1).OSArchitecture
  } catch {
    try { return (Get-WmiObject -Class Win32_OperatingSystem -ErrorAction Stop | Select-Object -First 1).OSArchitecture } catch {}
  }
  return $null
}

function Get-ChatGPTMCPProcessorMachineArchitecture {
  try {
    return (Get-CimInstance -ClassName Win32_Processor -ErrorAction Stop | Select-Object -First 1).Architecture
  } catch {
    try { return (Get-WmiObject -Class Win32_Processor -ErrorAction Stop | Select-Object -First 1).Architecture } catch {}
  }
  return $null
}

function Get-ChatGPTMCPRegistryProcessorIdentifier {
  try {
    return (Get-ItemProperty -Path 'HKLM:\HARDWARE\DESCRIPTION\System\CentralProcessor\0' -Name Identifier -ErrorAction Stop).Identifier
  } catch {}
  return $null
}

function Resolve-ChatGPTMCPArchitecture {
  param(
    [AllowNull()][string]$Override = $env:CHATGPT_MCP_ARCH,
    [AllowNull()][string]$RuntimeArchitecture = (Get-ChatGPTMCPRuntimeArchitecture),
    [AllowNull()][string]$ProcessorArchitectureW6432 = $env:PROCESSOR_ARCHITEW6432,
    [AllowNull()][string]$ProcessorArchitecture = $env:PROCESSOR_ARCHITECTURE,
    [AllowNull()][object]$ProcessorMachineArchitecture = (Get-ChatGPTMCPProcessorMachineArchitecture),
    [AllowNull()][string]$ProcessorIdentifier = $env:PROCESSOR_IDENTIFIER,
    [AllowNull()][string]$RegistryProcessorIdentifier = (Get-ChatGPTMCPRegistryProcessorIdentifier),
    [AllowNull()][string]$OSArchitecture = (Get-ChatGPTMCPOSArchitecture)
  )

  if ($Override) {
    $resolved = ConvertTo-ChatGPTMCPArchitecture $Override
    if ($resolved) { return $resolved }
    throw "chatgpt-mcp: unsupported CHATGPT_MCP_ARCH '$Override'; expected amd64 or arm64."
  }

  foreach ($candidate in @($RuntimeArchitecture, $ProcessorArchitectureW6432, $ProcessorArchitecture, $ProcessorIdentifier, $RegistryProcessorIdentifier, $OSArchitecture)) {
    $resolved = ConvertTo-ChatGPTMCPArchitecture $candidate
    if ($resolved) { return $resolved }
  }

  switch ([string]$ProcessorMachineArchitecture) {
    '9' { return 'amd64' }
    '12' { return 'arm64' }
  }

  # Win32_OperatingSystem commonly reports only "64-bit" on x64 Windows.
  # Keep this as the final compatibility fallback after all architecture-specific probes.
  if ($OSArchitecture -match '^\s*64[ -]?bit\s*$') { return 'amd64' }

  $diagnostics = @(
    "runtime='$RuntimeArchitecture'",
    "PROCESSOR_ARCHITEW6432='$ProcessorArchitectureW6432'",
    "PROCESSOR_ARCHITECTURE='$ProcessorArchitecture'",
    "processorMachine='$ProcessorMachineArchitecture'",
    "PROCESSOR_IDENTIFIER='$ProcessorIdentifier'",
    "registryIdentifier='$RegistryProcessorIdentifier'",
    "OSArchitecture='$OSArchitecture'"
  ) -join ', '
  throw "chatgpt-mcp: unsupported architecture; probes: $diagnostics. Set CHATGPT_MCP_ARCH=amd64 or arm64 to override."
}

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

$arch = Resolve-ChatGPTMCPArchitecture

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
