$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Repo = 'tridennote/cli'
$Binary = 'tridennote.exe'

if ($env:OS -ne 'Windows_NT') {
    Write-Host 'This installer is for Windows. On macOS or Linux run:'
    Write-Host "  curl -fsSL https://raw.githubusercontent.com/$Repo/main/install.sh | bash"
    return
}

try {
    [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
} catch {
}

$Arch = $env:TRIDENNOTE_ARCH
if (-not $Arch) {
    $Processor = $env:PROCESSOR_ARCHITEW6432
    if (-not $Processor) { $Processor = $env:PROCESSOR_ARCHITECTURE }
    switch ($Processor) {
        'AMD64' { $Arch = 'amd64' }
        'ARM64' { $Arch = 'arm64' }
        default { throw "Unsupported architecture: $Processor" }
    }
}

$Tag = $env:TRIDENNOTE_VERSION
if (-not $Tag) {
    $Release = Invoke-RestMethod -UseBasicParsing -Uri "https://api.github.com/repos/$Repo/releases/latest" -Headers @{ 'User-Agent' = 'tridennote-installer' }
    $Tag = $Release.tag_name
}
if (-not $Tag) { throw 'Could not find the latest tridennote release.' }

$Archive = "tridennote_windows_$Arch.zip"
$BaseUrl = "https://github.com/$Repo/releases/download/$Tag"
$TempDir = Join-Path ([IO.Path]::GetTempPath()) ("tridennote-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $TempDir | Out-Null

try {
    Write-Host "Installing tridennote $Tag for windows/$Arch..."
    $ArchivePath = Join-Path $TempDir $Archive
    $ChecksumsPath = Join-Path $TempDir 'checksums.txt'
    Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/$Archive" -OutFile $ArchivePath
    Invoke-WebRequest -UseBasicParsing -Uri "$BaseUrl/checksums.txt" -OutFile $ChecksumsPath

    $Expected = $null
    foreach ($Line in Get-Content $ChecksumsPath) {
        $Parts = $Line -split '\s+'
        if ($Parts.Length -ge 2 -and $Parts[1] -eq $Archive) { $Expected = $Parts[0].ToLowerInvariant() }
    }
    $Actual = (Get-FileHash -Algorithm SHA256 -Path $ArchivePath).Hash.ToLowerInvariant()
    if (-not $Expected -or $Expected -ne $Actual) { throw "Checksum mismatch for $Archive; aborting." }

    $Extracted = Join-Path $TempDir 'extracted'
    Expand-Archive -Path $ArchivePath -DestinationPath $Extracted -Force

    $InstallDir = $env:TRIDENNOTE_INSTALL_DIR
    if (-not $InstallDir) { $InstallDir = Join-Path $env:LOCALAPPDATA 'Programs\tridennote' }
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path (Join-Path $Extracted $Binary) -Destination (Join-Path $InstallDir $Binary) -Force
} finally {
    Remove-Item -Recurse -Force $TempDir -ErrorAction SilentlyContinue
}

$UserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
$Entries = @()
if ($UserPath) { $Entries = $UserPath -split ';' | Where-Object { $_ } }
if ($Entries -notcontains $InstallDir) {
    $NewPath = (($Entries + $InstallDir) -join ';')
    [Environment]::SetEnvironmentVariable('Path', $NewPath, 'User')
    Write-Host "Added $InstallDir to your user PATH. Open a new terminal to pick it up."
}
if (($env:Path -split ';') -notcontains $InstallDir) { $env:Path = "$env:Path;$InstallDir" }

Write-Host "Installed tridennote $Tag to $(Join-Path $InstallDir $Binary)"
Write-Host 'Run: tridennote'
