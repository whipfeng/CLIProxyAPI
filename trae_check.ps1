$procs = Get-Process | Where-Object { $_.ProcessName -like "*cli-proxy*" }
if ($procs) {
    $procs | Format-Table Id, ProcessName, StartTime -AutoSize
} else {
    Write-Host "No cli-proxy process found"
}

$src = Get-Item "C:\Users\Docker\vs-project\workspace\CLIProxyAPI\cli-proxy-win-amd64.exe"
$dst = Get-Item "C:\Users\Docker\Desktop\Workspace\proxy-ai-model\cli-proxy-win-amd64.exe"
Write-Host ("Source: " + $src.LastWriteTime + " Size: " + $src.Length)
Write-Host ("Dest:   " + $dst.LastWriteTime + " Size: " + $dst.Length)