---
description: "PowerShell scripting conventions for event-reactor. Cross-platform scripts (pwsh 7+), proper cmdlet usage, error handling, and output patterns. Use when writing or editing PowerShell files."
applyTo: "**/*.ps1"
---

# PowerShell Conventions

Scripts in this repo target **PowerShell 7+** (`pwsh`) and must work on
Windows, Linux, and macOS.

## Design Principle

Default to a **controller script** -- short, linear, and obvious. A script
should read like a recipe: get input, do the work, report results.

- Define helper functions **in the same script** if they make the controller
  logic clearer.
- If a function is **reusable across scripts**, extract to a separate file
  (dot-source) or module.
- The progression is: inline function -> separate file -> module. Don't
  over-engineer.
- **Never define functions inside other functions** -- they become untestable.

## Controller Script Template

~~~powershell
#Requires -Version 7.0

<#
.SYNOPSIS
    One-line summary.
.DESCRIPTION
    Detailed description.
.PARAMETER ParamName
    Parameter description.
.EXAMPLE
    PS> .\MyScript.ps1 -ParamName "value"
    Example output or description.
#>

[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [ValidateNotNullOrEmpty()]
    [string]$ParamName,

    [Parameter()]
    [ValidateSet('Option1', 'Option2')]
    [string]$Mode = 'Option1'
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# Controller logic -- short, linear, obvious
$data = Get-Data -Source $ParamName
$result = Invoke-Processing -Data $data -Mode $Mode
Write-Output $result
~~~

## Style

- **PascalCase** for function names and parameters
- **$camelCase** for local variables
- **Use full cmdlet names** -- never aliases (`Get-ChildItem` not `gci`,
  `ForEach-Object` not `%`, `Where-Object` not `?`)
- **Use named parameters** -- avoid positional parameters for readability
- **Approved verbs** -- function names must use `Verb-Noun` format with
  [approved verbs](https://learn.microsoft.com/en-us/powershell/scripting/developer/cmdlet/approved-verbs-for-windows-powershell-commands)

## Core Principles

- **PowerShell is object-oriented** -- output objects, not formatted strings.
  The caller decides how to format, filter, or export the result.
- **The pipeline passes objects, not text** -- design functions to accept and
  emit objects.
- **PowerShell has direct access to .NET** -- use `[ClassName]::Method()`
  syntax when the functionality is available.

## Cross-Platform

- Use `Join-Path` for path construction -- never hardcode `\` or `/`
- Use `$IsWindows`, `$IsLinux`, `$IsMacOS` for platform detection
- Use `[System.IO.Path]` methods for portable path operations
- Prefer PowerShell cmdlets over platform-specific binaries
  (`Get-ChildItem` not `ls`, `Remove-Item` not `rm`)
- Test with `pwsh` (PowerShell 7), not `powershell` (Windows PowerShell 5.1)

## Output Streams

Use the right cmdlet for the right stream:

~~~powershell
Write-Output "data"          # Pipeline output (consumed by callers)
Write-Verbose "detail"       # Diagnostic (shown with -Verbose)
Write-Warning "caution"      # Non-fatal issue
Write-Error "failed"         # Error stream
Write-Debug "state: $x"     # Deep diagnostics (shown with -Debug)
~~~

**Never use `Write-Host` for data** -- it cannot be captured, piped, or
redirected. `Write-Host` is only for messages meant for the human at the
keyboard -- never for data or status that a caller might consume.

Use `Write-Verbose` for progress/status messages. Use `Write-Output` (or
implicit output) for results.

## Error Handling

~~~powershell
# Wrap risky operations in try/finally for cleanup
try {
    $process = Start-Process -FilePath $binary -PassThru
    # ... work ...
} finally {
    if (-not $process.HasExited) {
        Stop-Process -Id $process.Id -Force
    }
}

# Check external command exit codes
& some-tool --flag
if ($LASTEXITCODE -ne 0) { throw "some-tool failed with exit code $LASTEXITCODE" }
~~~

## HTTP Requests

~~~powershell
# Use splatting for complex requests
$params = @{
    Uri         = "$baseUrl/api/endpoint"
    Method      = 'POST'
    ContentType = 'application/json'
    Body        = ($payload | ConvertTo-Json -Depth 10)
}
$response = Invoke-RestMethod @params

# Use Invoke-WebRequest when you need status codes and headers
$response = Invoke-WebRequest @params -UseBasicParsing
if ($response.StatusCode -ne 200) {
    throw "Unexpected status: $($response.StatusCode)"
}
~~~

## Parameter Validation

Use validation attributes to catch bad input early:

~~~powershell
[ValidateNotNullOrEmpty()]        # Rejects null and empty string
[ValidateRange(1, 100)]           # Numeric range
[ValidateSet('A', 'B', 'C')]      # Enumerated values
[ValidatePattern('^[a-z]+$')]     # Regex match
[ValidateScript({ Test-Path $_ })] # Custom validation
~~~

## Collections

~~~powershell
# Use typed lists for building collections (avoid += on arrays)
$list = [System.Collections.Generic.List[string]]::new()
$list.Add('item')

# Collect pipeline output
$results = foreach ($item in $collection) {
    Process-Item -Name $item
}
~~~

## Don'ts

- Don't use `Write-Host` for data output
- Don't use aliases in scripts (`gci`, `%`, `?`, `select`, `where`)
- Don't hardcode path separators
- Don't use `[System.IO.File]::WriteAllText()` when `Set-Content` suffices
- Don't suppress errors silently without logging (`-ErrorAction SilentlyContinue`
  is fine for `Test-Path` patterns, not for hiding failures)
