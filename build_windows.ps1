# Windows PowerShell 5.1+ • Go 1.23 required
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent $MyInvocation.MyCommand.Path
Push-Location (Join-Path $root 'app')
try {
  go test ./...
  if ($LASTEXITCODE -ne 0) { throw 'App tests failed' }
  go build -trimpath -ldflags '-s -w -H windowsgui' -o (Join-Path $root 'SlangDiscovery.exe') .
  if ($LASTEXITCODE -ne 0) { throw 'Desktop app build failed' }
} finally { Pop-Location }
Copy-Item (Join-Path $root 'SlangDiscovery.exe') (Join-Path $root 'installer\assets\SlangDiscovery.exe') -Force
Push-Location (Join-Path $root 'installer')
try {
  go build -trimpath -ldflags '-s -w -H windowsgui' -o (Join-Path $root 'Setup.exe') .
  if ($LASTEXITCODE -ne 0) { throw 'Installer build failed' }
} finally { Pop-Location }
Write-Host 'BUILD OK: SlangDiscovery.exe and Setup.exe'
