param(
    [Parameter(Mandatory = $true)]
    [string]$Output
)

$ErrorActionPreference = 'Stop'
$source = Get-Content plugins/bash/source.json -Raw | ConvertFrom-Json
$download = Join-Path $env:RUNNER_TEMP 'portable-git.7z.exe'
Invoke-WebRequest -Uri $source.url -OutFile $download
$actual = (Get-FileHash -Path $download -Algorithm SHA256).Hash.ToLowerInvariant()
if ($actual -ne $source.sha256) {
    throw "PortableGit SHA-256 mismatch: $actual"
}
$root = Join-Path $env:RUNNER_TEMP 'portable-git'
New-Item -ItemType Directory -Force -Path $root | Out-Null
& 7z x $download "-o$root" -y
if ($LASTEXITCODE -ne 0) { throw 'PortableGit extraction failed' }
Push-Location $root
try {
    & .\git-bash.exe --no-needs-console --hide --no-cd --command=post-install.bat
    if ($LASTEXITCODE -ne 0) { throw 'PortableGit post-install preparation failed' }
} finally {
    Pop-Location
}
$bash = Join-Path $root 'usr\bin\bash.exe'
$smoke = & $bash --noprofile --norc -lc 'test -n "$BASH_VERSION" && printf cgm-bash-build-smoke'
if ($LASTEXITCODE -ne 0 -or $smoke -ne 'cgm-bash-build-smoke') {
    throw "Prepared Bash runtime smoke test failed: $smoke"
}
go run ./plugins/bash/build --source-root $root --output $Output
if ($LASTEXITCODE -ne 0) { throw 'Bash plugin build failed' }
