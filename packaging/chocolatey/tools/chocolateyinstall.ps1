$ErrorActionPreference = 'Stop'

$toolsDir = Split-Path -Parent $MyInvocation.MyCommand.Definition
$release = 'https://github.com/tridennote/cli/releases/download/v__VERSION__'

$packageArgs = @{
  packageName    = $env:ChocolateyPackageName
  unzipLocation  = $toolsDir
  url64bit       = "$release/tridennote_windows_amd64.zip"
  checksum64     = '__CHECKSUM_AMD64__'
  checksumType64 = 'sha256'
}

if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') {
  $packageArgs.url64bit = "$release/tridennote_windows_arm64.zip"
  $packageArgs.checksum64 = '__CHECKSUM_ARM64__'
}

Install-ChocolateyZipPackage @packageArgs
