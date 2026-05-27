Get-Process | ForEach-Object {
    $n = $_.ProcessName.ToLower()
    if ($n.Contains('amd') -or $n.Contains('run') -or $n.Contains('win') -or $n.Contains('proxy') -or $n.Contains('cli')) {
        Write-Host "$($_.Id) $($_.ProcessName)"
    }
}