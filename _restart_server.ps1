$ErrorActionPreference = "SilentlyContinue"
$conn = Get-NetTCPConnection -LocalPort 8317 | Select-Object -First 1
if ($conn) {
    $proc = Get-Process -Id $conn.OwningProcess
    Write-Output "Killing: PID=$($proc.Id) Name=$($proc.Name)"
    Stop-Process -Id $proc.Id -Force
    Start-Sleep -Seconds 2
}

$exe = "C:\Users\Docker\Desktop\Workspace\proxy-ai-model\cli-proxy-win-amd64.exe"
$cfg = "C:\Users\Docker\Desktop\Workspace\proxy-ai-model\config.yaml"
$log = "C:\Users\Docker\Desktop\Workspace\proxy-ai-model\logs"
$static = "C:\Users\Docker\Desktop\Workspace\proxy-ai-model\static"

Start-Process -FilePath $exe -ArgumentList "--config","$cfg","--log-dir","$log","--static-dir","$static" -NoNewWindow
Start-Sleep -Seconds 3
Write-Output "Server started"