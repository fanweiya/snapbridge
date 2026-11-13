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

& bun install
if ($LASTEXITCODE -ne 0) { throw "bun install 失败，退出码 $LASTEXITCODE" }

& mygo build
if ($LASTEXITCODE -ne 0) { throw "mygo build 失败，退出码 $LASTEXITCODE" }

Write-Host "=== 产物:"
Get-ChildItem -Path "build" -Recurse -Filter *.exe | Select-Object FullName, Length
