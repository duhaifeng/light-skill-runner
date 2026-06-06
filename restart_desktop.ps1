$ErrorActionPreference = "Stop"

$RepoRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$Binary = Join-Path $RepoRoot "skill-desktop.exe"

Set-Location $RepoRoot

Get-Process -Name "skill-desktop" -ErrorAction SilentlyContinue | Stop-Process -Force

go build -tags "desktop,production" -o $Binary ./cmd/skill-desktop

Start-Process -FilePath $Binary -WorkingDirectory $RepoRoot
