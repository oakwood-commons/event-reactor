#Requires -Version 7.0

<#
.SYNOPSIS
    Smoke-tests event-reactor by starting the server and sending sample events.

.DESCRIPTION
    Builds the binary, starts the server in the background, waits for it to
    become healthy, fires sample events at every endpoint, then tears it down.
    Works on Windows, Linux, and macOS (PowerShell 7+).

.PARAMETER Port
    Port for the smoke-test server. Defaults to 8088.

.PARAMETER SkipBuild
    Skip the build step (use existing binary).

.EXAMPLE
    pwsh scripts/helper.ps1

.EXAMPLE
    pwsh scripts/helper.ps1 -Port 9090 -SkipBuild
#>

[CmdletBinding()]
param(
    [Parameter()]
    [ValidateRange(1024, 65535)]
    [int]$Port = 8088,

    [Parameter()]
    [switch]$SkipBuild
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# ── Helpers ──────────────────────────────────────────────────────────────────

function Test-Endpoint {
    [CmdletBinding()]
    param(
        [Parameter(Mandatory)]
        [string]$Label,

        [Parameter(Mandatory)]
        [hashtable]$RequestParams,

        [Parameter()]
        [int]$ExpectedStatus = 200,

        [Parameter()]
        [int]$MinProcessed = 0
    )

    $response = Invoke-WebRequest @RequestParams -UseBasicParsing

    $result = [PSCustomObject]@{
        Label      = $Label
        StatusCode = $response.StatusCode
        Expected   = $ExpectedStatus
        Passed     = $response.StatusCode -eq $ExpectedStatus
        Processed  = $null
    }

    if ($MinProcessed -gt 0 -and $result.Passed) {
        $body = $response.Content | ConvertFrom-Json
        $result.Processed = [int]$body.processed
        if ($body.processed -lt $MinProcessed) {
            $result.Passed = $false
        }
    }

    $result
}

# ── Configuration ────────────────────────────────────────────────────────────

$repoRoot = Split-Path -Parent $PSScriptRoot
$configFile = Join-Path $PSScriptRoot 'smoke-config.yaml'
$baseUrl = "http://localhost:$Port"

if ($IsWindows -or $env:OS -match 'Windows') {
    $binary = Join-Path $repoRoot 'dist' 'er.exe'
} else {
    $binary = Join-Path $repoRoot 'dist' 'er'
}

# ── Build ────────────────────────────────────────────────────────────────────

if (-not $SkipBuild) {
    Write-Verbose 'Building event-reactor...'
    $distDir = Split-Path $binary
    if (-not (Test-Path -Path $distDir)) {
        New-Item -ItemType Directory -Force -Path $distDir | Out-Null
    }
    Push-Location $repoRoot
    try {
        & go build -o $binary ./cmd/er
        if ($LASTEXITCODE -ne 0) { throw 'Build failed' }
        Write-Verbose "Built $binary"
    } finally {
        Pop-Location
    }
}

if (-not (Test-Path -Path $binary)) {
    throw "Binary not found at $binary -- run without -SkipBuild"
}

# ── Start server ─────────────────────────────────────────────────────────────

Write-Verbose "Starting server on port $Port..."
$startArgs = @{
    FilePath     = $binary
    ArgumentList = 'run', 'server', '--config', $configFile
    PassThru     = $true
}
if ($IsWindows) {
    $startArgs['NoNewWindow'] = $true
} else {
    $startArgs['RedirectStandardOutput'] = Join-Path ([System.IO.Path]::GetTempPath()) 'er-stdout.log'
    $startArgs['RedirectStandardError']  = Join-Path ([System.IO.Path]::GetTempPath()) 'er-stderr.log'
}
$serverProcess = Start-Process @startArgs

Start-Sleep -Seconds 2

if ($serverProcess.HasExited) {
    throw "Server exited immediately with code $($serverProcess.ExitCode)"
}

# ── Run smoke tests ──────────────────────────────────────────────────────────

$results = @()

try {
    # Health checks
    $results += Test-Endpoint -Label 'Liveness' -RequestParams @{
        Uri    = "$baseUrl/health/live"
        Method = 'GET'
    }

    $results += Test-Endpoint -Label 'Readiness' -RequestParams @{
        Uri    = "$baseUrl/health/ready"
        Method = 'GET'
    }

    # POST /events
    $results += Test-Endpoint -Label '/events' -MinProcessed 1 -RequestParams @{
        Uri         = "$baseUrl/events"
        Method      = 'POST'
        ContentType = 'application/json'
        Body        = (@{
            action    = 'test.smoke'
            message   = 'Hello from smoke test'
            timestamp = (Get-Date -Format o)
        } | ConvertTo-Json -Depth 10)
    }

    # POST /cloudevents (structured)
    $results += Test-Endpoint -Label '/cloudevents (structured)' -RequestParams @{
        Uri         = "$baseUrl/cloudevents"
        Method      = 'POST'
        ContentType = 'application/json'
        Body        = (@{
            specversion = '1.0'
            id          = 'smoke-ce-001'
            source      = 'smoke-test'
            type        = 'com.example.smoke'
            data        = @{
                action = 'cloud_event_test'
                detail = 'structured content mode'
            }
        } | ConvertTo-Json -Depth 10)
    }

    # POST /cloudevents (binary content mode)
    $results += Test-Endpoint -Label '/cloudevents (binary)' -RequestParams @{
        Uri         = "$baseUrl/cloudevents"
        Method      = 'POST'
        ContentType = 'application/json'
        Headers     = @{
            'Ce-Specversion' = '1.0'
            'Ce-Id'          = 'smoke-ce-002'
            'Ce-Source'      = 'smoke-test-binary'
            'Ce-Type'        = 'com.example.smoke.binary'
        }
        Body        = (@{ detail = 'binary content mode' } | ConvertTo-Json -Depth 10)
    }

    # POST /webhook/:source
    $results += Test-Endpoint -Label '/webhook/smoke' -MinProcessed 1 -RequestParams @{
        Uri         = "$baseUrl/webhook/smoke"
        Method      = 'POST'
        ContentType = 'application/json'
        Headers     = @{ 'X-Event-Type' = 'push' }
        Body        = (@{
            action     = 'webhook_test'
            repository = @{ full_name = 'oakwood-commons/event-reactor' }
        } | ConvertTo-Json -Depth 10)
    }
} finally {
    Write-Verbose 'Stopping server...'
    if (-not $serverProcess.HasExited) {
        Stop-Process -Id $serverProcess.Id -Force
        Write-Verbose 'Server stopped'
    } else {
        Write-Warning "Server already exited (code $($serverProcess.ExitCode))"
    }
}

# ── Results ──────────────────────────────────────────────────────────────────

$failures = $results | Where-Object { -not $_.Passed }

Write-Output $results

if ($failures) {
    Write-Error "$($failures.Count) check(s) failed"
}

Write-Verbose 'All checks passed'
