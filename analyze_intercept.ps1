param(
    [string]$FilePath
)

$ErrorActionPreference = "Stop"

$content = Get-Content -Path $FilePath -Raw -Encoding UTF8
# Strip markdown header: find the first '['
$idx = $content.IndexOf('[')
if ($idx -gt 0) { $content = $content.Substring($idx) }

$json = $content | ConvertFrom-Json

Write-Host "=== TOTAL REQUESTS: $($json.Count) ==="
Write-Host ""

# 1a. TASK 1: List all URLs
Write-Host "========== TASK 1: ALL REQUESTS SUMMARY =========="
$i = 0
foreach ($req in $json) {
    $i++
    $ua = $req.req.headers.'user-agent'
    $ttfb = $req.ttfb
    $startTime = $req.startTime
    $url = $req.url
    # Determine type
    $type = if ($ua -like '*Go-http-client*') { 'CLIProxy' } else { 'RealClient' }
    Write-Host "[$i] TYPE=$type | TTFB=${ttfb}ms | startTime=$startTime"
    Write-Host "    URL: $url"
    Write-Host "    UA : $ua"
    Write-Host ""
}

# 1b. Group by type
Write-Host "========== REQUEST COUNT BY TYPE =========="
$proxyCount = ($json | Where-Object { $_.req.headers.'user-agent' -like '*Go-http-client*' }).Count
$realCount = ($json | Where-Object { $_.req.headers.'user-agent' -notlike '*Go-http-client*' }).Count
Write-Host "CLIProxy (Go-http-client): $proxyCount"
Write-Host "RealClient (TraeClient):   $realCount"
Write-Host ""

# TASK 2: Check rawHeaderNames for x-routing-weight
Write-Host "========== TASK 2: X-ROUTING-WEIGHT HEADER CHECK =========="
$routingWeightRequests = @()
foreach ($req in $json) {
    $rawNames = $req.req.rawHeaderNames
    $found = $false
    if ($rawNames) {
        foreach ($key in $rawNames.PSObject.Properties.Name) {
            if ($key -like '*routing*weight*' -or $key -like '*Routing*Weight*' -or $key -like '*ROUTING*WEIGHT*') {
                $found = $true
                Write-Host "FOUND: $key => $($rawNames.$key)"
                Write-Host "  URL: $($req.url)"
                Write-Host "  UA : $($req.req.headers.'user-agent')"
                $routingWeightRequests += $req
            }
        }
    }
}
if ($routingWeightRequests.Count -eq 0) {
    Write-Host "No 'x-routing-weight' header found in any request."
}
Write-Host ""

# TASK 3: Detailed info for X-Routing-Weight requests
Write-Host "========== TASK 3: X-ROUTING-WEIGHT REQUESTS DETAIL =========="
if ($routingWeightRequests.Count -eq 0) {
    Write-Host "N/A - No such requests."
} else {
    foreach ($req in $routingWeightRequests) {
        Write-Host "---"
        Write-Host "URL       : $($req.url)"
        Write-Host "UA        : $($req.req.headers.'user-agent')"
        Write-Host "TTFB      : $($req.ttfb)"
        Write-Host "startTime : $($req.startTime)"
        Write-Host "RawHeaders:"
        $req.req.rawHeaderNames.PSObject.Properties | ForEach-Object { Write-Host "  $($_.Name) = $($_.Value)" }
        Write-Host "Headers   :"
        $req.req.headers.PSObject.Properties | ForEach-Object { 
            $val = $_.Value
            if ($val.Length -gt 200) { $val = $val.Substring(0,200) + '...' }
            Write-Host "  $($_.Name) = $val"
        }
        Write-Host "Extra (decoded):"
        try {
            $extraStr = $req.req.headers.'extra'
            if ($extraStr) {
                $extraObj = $extraStr | ConvertFrom-Json
                $extraObj.PSObject.Properties | ForEach-Object { Write-Host "  $($_.Name) = $($_.Value)" }
            }
        } catch { Write-Host "  (could not parse Extra)" }
    }
}
Write-Host ""

# TASK 4: RealClient requests - full header list, Extra, body structure
Write-Host "========== TASK 5: REAL CLIENT REQUESTS (non Go-http-client) =========="
$realClients = $json | Where-Object { $_.req.headers.'user-agent' -notlike '*Go-http-client*' }
if ($realClients.Count -eq 0) {
    Write-Host "No real client requests found."
} else {
    $rc_i = 0
    foreach ($req in $realClients) {
        $rc_i++
        Write-Host "--- RealClient #$rc_i ---"
        Write-Host "URL       : $($req.url)"
        Write-Host "Method    : $($req.req.method)"
        Write-Host "TTFB      : $($req.ttfb)"
        Write-Host "startTime : $($req.startTime)"
        Write-Host "useH2     : $($req.useH2)"
        Write-Host ""

        Write-Host "--- rawHeaderNames (complete list):"
        if ($req.req.rawHeaderNames) {
            $req.req.rawHeaderNames.PSObject.Properties | ForEach-Object { Write-Host "  $($_.Name) = $($_.Value)" }
        }
        Write-Host ""

        Write-Host "--- Headers (complete list):"
        if ($req.req.headers) {
            $req.req.headers.PSObject.Properties | ForEach-Object { 
                $val = if ($_.Value -is [string]) { $_.Value } else { "$($_.Value)" }
                if ($val.Length -gt 500) { $val = $val.Substring(0,500) + "...[TRUNCATED]" }
                Write-Host "  $($_.Name) = $val"
            }
        }
        Write-Host ""

        Write-Host "--- Extra (decoded JSON):"
        try {
            $extraStr = $req.req.headers.'extra'
            if ($extraStr) {
                $extraObj = $extraStr | ConvertFrom-Json
                $extraObj.PSObject.Properties | ForEach-Object { Write-Host "  $($_.Name) = $($_.Value)" }
            } else {
                Write-Host "  (no Extra header)"
            }
        } catch { 
            Write-Host "  (could not parse Extra) Raw: $extraStr"
        }
        Write-Host ""

        Write-Host "--- Request Body Structure:"
        $body = $req.req.body
        if (-not $body -or $body -eq '') {
            # Try base64
            $base64 = $req.req.base64
            if ($base64) {
                Write-Host "  Body is base64 encoded, length=$($base64.Length)"
                try {
                    $decoded = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($base64))
                    if ($decoded.Length -gt 2000) { $decoded = $decoded.Substring(0,2000) + "...[TRUNCATED]" }
                    Write-Host "  Decoded body:"
                    Write-Host $decoded
                    # Try parse as JSON
                    try {
                        $bodyObj = $decoded | ConvertFrom-Json
                        Write-Host ""
                        Write-Host "  Body JSON structure (top-level keys):"
                        $bodyObj.PSObject.Properties.Name | ForEach-Object { Write-Host "    - $_" }
                    } catch { Write-Host "  (body not valid JSON)" }
                } catch { Write-Host "  (failed to decode base64)" }
            } else {
                Write-Host "  (no body captured)"
            }
        } else {
            if ($body.Length -gt 2000) { $body = $body.Substring(0,2000) + "...[TRUNCATED]" }
            Write-Host $body
            try {
                $bodyObj = $body | ConvertFrom-Json
                Write-Host ""
                Write-Host "  Body JSON structure (top-level keys):"
                $bodyObj.PSObject.Properties.Name | ForEach-Object { Write-Host "    - $_" }
            } catch { Write-Host "  (body not valid JSON)" }
        }
        Write-Host ""
    }
}

Write-Host "========== ANALYSIS COMPLETE =========="