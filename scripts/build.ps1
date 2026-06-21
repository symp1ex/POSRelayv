param(
    [switch]$InstallMissingTools,
    [switch]$WindowsGui
)

$ErrorActionPreference = "Stop"

function Test-Command {
    param([Parameter(Mandatory = $true)][string]$Name)

    return $null -ne (Get-Command $Name -ErrorAction SilentlyContinue)
}

function Add-PathForCurrentProcess {
    param([Parameter(Mandatory = $true)][string]$PathToAdd)

    if (!(Test-Path $PathToAdd)) {
        return
    }

    $parts = $env:Path -split ';'
    if ($parts -notcontains $PathToAdd) {
        $env:Path = "$PathToAdd;$env:Path"
    }
}

function Ensure-Go {
    if (!(Test-Command go)) {
        throw "go не найден в PATH. Установите Go и перезапустите PowerShell / GoLand."
    }
}

function Ensure-Node {
    if (!(Test-Command npm)) {
        throw "npm не найден в PATH. Установите Node.js и перезапустите PowerShell / GoLand."
    }
}

function Ensure-MSYS2-UCRT64 {
    $msysRoot = "C:\msys64"
    $ucrtBin = Join-Path $msysRoot "ucrt64\bin"
    $bash = Join-Path $msysRoot "usr\bin\bash.exe"

    Add-PathForCurrentProcess $ucrtBin

    if (Test-Command gcc) {
        Write-Host "gcc found:"
        gcc --version | Select-Object -First 1
        return
    }

    Write-Host ""
    Write-Host "gcc was not found. Installing MSYS2 and UCRT64 toolchain..."

    if (!(Test-Command winget)) {
        throw "winget was not found. Install MSYS2 manually from https://www.msys2.org/"
    }

    if (!(Test-Path $msysRoot)) {
        Write-Host ""
        Write-Host "== Installing MSYS2 =="
        winget install --id MSYS2.MSYS2 -e --accept-package-agreements --accept-source-agreements
    }

    if (!(Test-Path $bash)) {
        throw "MSYS2 bash.exe was not found: $bash"
    }

    Write-Host ""
    Write-Host "== Installing MSYS2 UCRT64 toolchain =="

    & $bash -lc "pacman --noconfirm -Sy"
    & $bash -lc "pacman --noconfirm -S --needed mingw-w64-ucrt-x86_64-toolchain"

    Add-PathForCurrentProcess $ucrtBin

    if (!(Test-Command gcc)) {
        throw "gcc is still not available. Check this path: $ucrtBin"
    }

    Write-Host "gcc installed:"
    gcc --version | Select-Object -First 1
}

$Root = Resolve-Path "$PSScriptRoot\.."
$FrontendDir = Join-Path $Root "ui\rd-web"
$BinDir = Join-Path $Root "bin"
$OutputExe = Join-Path $BinDir "POSRelayv.exe"

Write-Host "== POSRelayv build =="
Write-Host "Root: $Root"

if (!(Test-Path $FrontendDir)) {
    throw "Frontend directory not found: $FrontendDir"
}

Ensure-Go
Ensure-Node
Ensure-MSYS2-UCRT64

$env:CGO_ENABLED = "1"

Write-Host ""
Write-Host "== Build environment =="
go env GOOS GOARCH CGO_ENABLED CC CXX
gcc --version | Select-Object -First 1

Write-Host ""
Write-Host "== Installing frontend dependencies =="
Push-Location $FrontendDir
try {
    npm install

    Write-Host ""
    Write-Host "== Building React UI =="
    npm run build
}
finally {
    Pop-Location
}

$DistIndex = Join-Path $Root "internal\gui\web\dist\index.html"
if (!(Test-Path $DistIndex)) {
    throw "React build failed: $DistIndex not found"
}

finally {
    Pop-Location
}

Write-Host ""
Write-Host "Build completed:"
Write-Host $OutputExe