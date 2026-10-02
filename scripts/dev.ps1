<#
.SYNOPSIS
  Developer tasks on Windows (the Makefile's equivalents).

.DESCRIPTION
    .\scripts\dev.ps1 build      build\whatsapp-doppel.exe (console build: logs show in the terminal)
    .\scripts\dev.ps1 run        run for real with .\data and open the browser
    .\scripts\dev.ps1 dev-fake   simulated WhatsApp + canned LLM with .\data\fake
    .\scripts\dev.ps1 test       go test -race ./...  (needs a C compiler for -race; falls back to no race)
    .\scripts\dev.ps1 vet        go vet + gofmt check
    .\scripts\dev.ps1 fmt        gofmt -w
    .\scripts\dev.ps1 gui        build\WhatsappDoppel.exe without a console window (like the release)
    .\scripts\dev.ps1 install    build and install for this user (scripts\install-from-source.ps1)
    .\scripts\dev.ps1 smoke      end-to-end smoke test (needs Git Bash and jq)
  Plain Go works too:  go run . serve --open   |   go run . serve --fake-wa --fake-llm --data-dir .\data\fake --open
#>
param([Parameter(Position = 0)][string]$Task = 'help')
Set-StrictMode -Version 2.0
$ErrorActionPreference = 'Stop'
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location -LiteralPath $root

$old = $ErrorActionPreference; $ErrorActionPreference = 'Continue'
$version = (& git describe --tags --always --dirty 2>$null | Out-String).Trim()
$ErrorActionPreference = $old
if (-not $version) { $version = 'dev' }
$ldflags = "-X main.version=$version -X main.devProjectDir=$root"
$bin = Join-Path $root 'build\whatsapp-doppel.exe'

function Run([string]$file, [string[]]$arguments) {
  $old = $ErrorActionPreference
  $ErrorActionPreference = 'Continue'
  try { & $file @arguments; $code = $LASTEXITCODE } finally { $ErrorActionPreference = $old }
  if ($code -ne 0) { throw "$file $($arguments -join ' ') failed (exit $code)" }
}
function Build {
  $env:CGO_ENABLED = '0'
  Run 'go' @('build', '-trimpath', '-ldflags', $ldflags, '-o', $bin, '.')
  Remove-Item Env:\CGO_ENABLED
}

switch ($Task) {
  'build' { Build }
  'run' { Build; Run $bin @('serve', '--data-dir', (Join-Path $root 'data'), '--open') }
  'dev-fake' { Build; Run $bin @('serve', '--fake-wa', '--fake-llm', '--data-dir', (Join-Path $root 'data\fake'), '--open') }
  'test' {
    if (Get-Command gcc -ErrorAction SilentlyContinue) { Run 'go' @('test', '-race', './...') }
    else { Write-Warning 'gcc not found: running tests without the race detector'; Run 'go' @('test', './...') }
  }
  'vet' {
    Run 'go' @('vet', './...')
    $out = (& gofmt -l main.go internal scripts web | Out-String).Trim()
    if ($out) { Write-Host "gofmt needed:`n$out"; exit 1 }
  }
  'fmt' { Run 'gofmt' @('-w', 'main.go', 'internal', 'scripts', 'web') }
  'gui' {
    $env:CGO_ENABLED = '0'
    Run 'go' @('build', '-trimpath', '-ldflags', "-s -w -H=windowsgui $ldflags", '-o', (Join-Path $root 'build\WhatsappDoppel.exe'), '.')
    Remove-Item Env:\CGO_ENABLED
  }
  'install' { & (Join-Path $root 'scripts\install-from-source.ps1'); exit $LASTEXITCODE }
  'smoke' {
    $bash = Get-Command bash -ErrorAction SilentlyContinue
    if (-not $bash) { throw 'bash not found: install Git for Windows (Git Bash) to run the smoke test' }
    Run $bash.Source @('scripts/smoke.sh')
  }
  default { Get-Content -LiteralPath (Join-Path $root 'scripts\dev.ps1') -TotalCount 15 | Select-Object -Skip 5 | Out-Host }
}
