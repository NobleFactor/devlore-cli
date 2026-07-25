# PowerShell: Initialize SSH identity on Windows
$sshDir = "$env:USERPROFILE\.ssh"
if (-not (Test-Path $sshDir)) {
    New-Item -ItemType Directory -Path $sshDir
}
ssh-keygen -t ed25519 -f "$sshDir\id_ed25519"
