$ErrorActionPreference = "Stop"
Set-Location (Split-Path -Parent $MyInvocation.MyCommand.Path)
$env:Path = "C:\Users\Lenovo\sdk\go\bin;C:\Users\Lenovo\go\bin;C:\Users\Lenovo\.bun\bin;" + $env:Path
$env:GOPROXY = "https://goproxy.cn,direct"

& "C:\Users\Lenovo\go\bin\mygo.exe" build
if ($LASTEXITCODE -ne 0) { throw "mygo build 失败，退出码 $LASTEXITCODE" }

Write-Host "=== 产物:"
Get-ChildItem -Path "build" -Recurse -Filter *.exe | Select-Object FullName, Length
