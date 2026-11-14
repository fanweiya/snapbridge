$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $MyInvocation.MyCommand.Path)

# 把 Go、Bun 与 go install 产物的常见安装位置补进 PATH，
# 这样在没配置过环境变量的机器上也能直接构建。
$candidates = @(
  (Join-Path $env:USERPROFILE "sdk\go\bin"),
  (Join-Path $env:USERPROFILE "go\bin"),
  (Join-Path $env:USERPROFILE ".bun\bin"),
  "C:\Program Files\Go\bin"
)
foreach ($dir in $candidates) {
  if ((Test-Path $dir) -and ($env:Path -notlike "*$dir*")) {
    $env:Path = "$dir;$env:Path"
  }
}

foreach ($tool in @("go", "bun", "mygo")) {
  if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
    throw "找不到 $tool。请先安装 Go 1.27+、Bun 1.3+，并执行 go install github.com/egoist/mygo/cmd/mygo@latest"
  }
}

if (-not $env:GOPROXY) { $env:GOPROXY = "https://proxy.golang.org,direct" }

# 更新包必须用 mygo keygen 生成的私钥签名，装好的应用才会接受。
# 私钥留在 %APPDATA%\mygo\update-keys\ 下，不进仓库，也不打印到日志。
if (-not $env:MYGO_UPDATER_PRIVATE_KEY) {
  $keyFile = Join-Path $env:APPDATA "mygo\update-keys\mygo-update.key"
  if (Test-Path $keyFile) {
    $env:MYGO_UPDATER_PRIVATE_KEY = (Get-Content $keyFile -Raw).Trim()
  } else {
    Write-Warning "找不到更新私钥 $keyFile，本次构建不会产出更新包。先运行 mygo keygen。"
  }
}

& bun install
if ($LASTEXITCODE -ne 0) { throw "bun install 失败，退出码 $LASTEXITCODE" }

& mygo build
if ($LASTEXITCODE -ne 0) { throw "mygo build 失败，退出码 $LASTEXITCODE" }

Write-Host "=== 产物:"
Get-ChildItem -Path "build" -Recurse -Filter *.exe | Select-Object FullName, Length
