$ErrorActionPreference = "Stop"

$ApiBaseUrl = if ($env:KTUI_API_BASE_URL) { $env:KTUI_API_BASE_URL } else { "https://gitea.bytevibe.dev/api/v1/repos/gary/ktui" }
$InstallDir = if ($env:KTUI_INSTALL_DIR) { $env:KTUI_INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "ktui\bin" }
$Version = if ($env:KTUI_VERSION) { $env:KTUI_VERSION } else { "latest" }
$BinaryName = "ktui.exe"

function Fail($Message) {
    Write-Error "ktui install: $Message"
    exit 1
}

function Invoke-KtuiGet($Url, $OutFile) {
    $headers = @{}
    if ($env:KTUI_UPDATE_TOKEN) {
        $headers["Authorization"] = "token $env:KTUI_UPDATE_TOKEN"
    } elseif ($env:GITEA_TOKEN) {
        $headers["Authorization"] = "token $env:GITEA_TOKEN"
    }
    if ($OutFile) {
        Invoke-WebRequest -UseBasicParsing -Headers $headers -Uri $Url -OutFile $OutFile
    } else {
        Invoke-RestMethod -Headers $headers -Uri $Url
    }
}

function Get-KtuiArch {
    switch ($env:PROCESSOR_ARCHITECTURE) {
        "AMD64" { "amd64"; break }
        "ARM64" { "arm64"; break }
        default { Fail "unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
    }
}

$endpoint = "$ApiBaseUrl/releases/latest"
if ($Version -ne "latest") {
    $endpoint = "$ApiBaseUrl/releases/tags/$Version"
}

$release = Invoke-KtuiGet $endpoint
if (-not $release.tag_name) {
    Fail "release response does not include tag_name"
}

$arch = Get-KtuiArch
$suffix = "_windows_${arch}.zip"
$asset = $release.assets | Where-Object {
    $_.name -like "ktui_*" -and $_.name.EndsWith($suffix) -and $_.browser_download_url
} | Select-Object -First 1
$checksums = $release.assets | Where-Object {
    $_.name -eq "checksums.txt" -and $_.browser_download_url
} | Select-Object -First 1

if (-not $asset) {
    Fail "release $($release.tag_name) does not contain an asset for windows/$arch"
}
if (-not $checksums) {
    Fail "release does not contain checksums.txt"
}

$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("ktui-install-" + [System.Guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Path $tmp | Out-Null
try {
    $archivePath = Join-Path $tmp $asset.name
    $checksumsPath = Join-Path $tmp "checksums.txt"

    Write-Host "ktui install: downloading $($asset.name)"
    Invoke-KtuiGet $asset.browser_download_url $archivePath
    Invoke-KtuiGet $checksums.browser_download_url $checksumsPath

    $expected = $null
    foreach ($line in Get-Content $checksumsPath) {
        $parts = $line -split "\s+"
        if ($parts.Length -lt 2) {
            continue
        }
        $filename = Split-Path ($parts[-1].TrimStart("*")) -Leaf
        if ($filename -eq $asset.name) {
            $expected = $parts[0].ToLowerInvariant()
            break
        }
    }
    if (-not $expected) {
        Fail "checksums.txt does not contain $($asset.name)"
    }

    $got = (Get-FileHash -Algorithm SHA256 $archivePath).Hash.ToLowerInvariant()
    if ($got -ne $expected) {
        Fail "checksum mismatch for $($asset.name)"
    }

    Expand-Archive -Force -Path $archivePath -DestinationPath $tmp
    $binary = Get-ChildItem -Path $tmp -Recurse -File -Filter $BinaryName | Select-Object -First 1
    if (-not $binary) {
        Fail "archive does not contain $BinaryName"
    }

    New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
    $installPath = Join-Path $InstallDir $BinaryName
    Copy-Item -Force $binary.FullName $installPath

    Write-Host "ktui install: installed $installPath"
    $pathEntries = $env:PATH -split ";"
    if ($pathEntries -notcontains $InstallDir) {
        Write-Host "ktui install: add $InstallDir to PATH if ktui is not found"
    }
    & $installPath version
} finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}
