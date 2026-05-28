# Backend: clean untracked junk, then stage all modified/added source files
$exclude = @(
    'node_modules', '*.png', '*.ps1', '*.py', '*.cjs', '.js',
    'zhuabao/', 'tmp/', 'whistle_data.json', 'deploy.js',
    'debug*', 'final_step*', 'feishu*', 'check_*', 'analyze_*',
    'extract_*', 'decode_*', 'read_*', 'fetch_*', 'find_proc.*',
    'trae_check.*', '_*', '$null', 'commit-msg.txt',
    'package.json', 'package-lock.json'
)
# Remove untracked junk
Get-ChildItem -Recurse -File | Where-Object {
    $rel = $_.FullName.Substring((Get-Location).Path.Length + 1)
    $untracked = (git status $rel 2>&1) -match '^\?\?'
    if ($untracked) {
        foreach ($p in $exclude) { if ($rel -like $p -or $rel -like ($p + '\*')) { return $true } }
    }
    return $false
} | ForEach-Object { Remove-Item $_.FullName -Force }

Write-Output "Junk cleaned"
git add internal/ cmd/ sdk/ examples/ test/ AGENTS.md *.go 2>&1 | Out-Null
Write-Output ("Staged: $(git diff --cached --name-only.Count) files")
