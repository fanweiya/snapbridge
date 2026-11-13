# SnapBridge 冒烟测试：验证热键唤起、选区拖拽、双通道剪贴板写入。
# 用法：powershell -ExecutionPolicy Bypass -File scripts\smoke-test.ps1
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$exe = Join-Path $root "build\windows-amd64\SnapBridge.exe"
$out = Join-Path $root "build\smoke"
New-Item -ItemType Directory -Force -Path $out | Out-Null

Add-Type -TypeDefinition @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class SB {
  public delegate bool EnumProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc cb, IntPtr l);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern int GetClassNameW(IntPtr h, StringBuilder s, int n);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
  [DllImport("user32.dll")] public static extern void mouse_event(uint f, uint dx, uint dy, uint d, IntPtr e);
  [DllImport("user32.dll")] public static extern void keybd_event(byte b, byte s, uint f, IntPtr e);
  public struct RECT { public int Left, Top, Right, Bottom; }

  public static string Windows(uint pid) {
    var sb = new StringBuilder();
    EnumWindows(delegate(IntPtr h, IntPtr l) {
      uint p; GetWindowThreadProcessId(h, out p);
      if (p == pid && IsWindowVisible(h)) {
        var c = new StringBuilder(256); GetClassNameW(h, c, 256);
        RECT r; GetWindowRect(h, out r);
        sb.AppendLine(String.Format("{0} ({1},{2})-({3},{4}) {5}x{6}", c, r.Left, r.Top, r.Right, r.Bottom, r.Right-r.Left, r.Bottom-r.Top));
      }
      return true;
    }, IntPtr.Zero);
    return sb.ToString().Trim();
  }

  public static void Key(byte vk) {
    keybd_event(vk, 0, 0, IntPtr.Zero);
    System.Threading.Thread.Sleep(60);
    keybd_event(vk, 0, 2, IntPtr.Zero);
  }

  public static void Drag(int x1, int y1, int x2, int y2) {
    SetCursorPos(x1, y1);
    System.Threading.Thread.Sleep(120);
    mouse_event(0x0002, 0, 0, 0, IntPtr.Zero);
    System.Threading.Thread.Sleep(80);
    int steps = 24;
    for (int i = 1; i <= steps; i++) {
      SetCursorPos(x1 + (x2 - x1) * i / steps, y1 + (y2 - y1) * i / steps);
      System.Threading.Thread.Sleep(12);
    }
    System.Threading.Thread.Sleep(120);
    mouse_event(0x0004, 0, 0, 0, IntPtr.Zero);
  }
}
"@

$proc = Get-Process -Name "SnapBridge" -ErrorAction SilentlyContinue
if (-not $proc) {
  Start-Process -FilePath $exe
  Start-Sleep -Seconds 3
  $proc = Get-Process -Name "SnapBridge"
}
Write-Host "[1] 进程 PID = $($proc.Id)"
Write-Host "    后台窗口: '$([SB]::Windows([uint32]$proc.Id))'"

Write-Host "[2] 发送 PrintScreen"
[SB]::Key(0x2C)
Start-Sleep -Milliseconds 1500
$wins = [SB]::Windows([uint32]$proc.Id)
Write-Host "    遮罩窗口: $wins"
if (-not $wins) { throw "热键没有唤起遮罩窗口" }

Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$screen = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
Write-Host "    主屏: $($screen.Width)x$($screen.Height)"

$shot = New-Object System.Drawing.Bitmap $screen.Width, $screen.Height
$g = [System.Drawing.Graphics]::FromImage($shot)
$g.CopyFromScreen(0, 0, 0, 0, $shot.Size)
$shot.Save((Join-Path $out "01-overlay.png"), [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose(); $shot.Dispose()

Write-Host "[3] 拖拽选区 400,300 -> 900,700"
[SB]::Drag(400, 300, 900, 700)
Start-Sleep -Milliseconds 600

$shot2 = New-Object System.Drawing.Bitmap $screen.Width, $screen.Height
$g2 = [System.Drawing.Graphics]::FromImage($shot2)
$g2.CopyFromScreen(0, 0, 0, 0, $shot2.Size)
$shot2.Save((Join-Path $out "02-selected.png"), [System.Drawing.Imaging.ImageFormat]::Png)
$g2.Dispose(); $shot2.Dispose()

Write-Host "[4] 回车复制"
[SB]::Key(0x0D)
Start-Sleep -Milliseconds 1200
Write-Host "    遮罩窗口: '$([SB]::Windows([uint32]$proc.Id))'"

$text = Get-Clipboard -Raw -ErrorAction SilentlyContinue
Write-Host "    剪贴板文本: $text"
$img = Get-Clipboard -Format Image -ErrorAction SilentlyContinue
if ($img) {
  Write-Host "    剪贴板位图: $($img.Width)x$($img.Height)"
  $img.Save((Join-Path $out "03-clipboard.png"), [System.Drawing.Imaging.ImageFormat]::Png)
  $img.Dispose()
} else {
  Write-Host "    剪贴板位图: 无"
}
if ($text -and (Test-Path $text)) {
  $f = Get-Item $text
  Add-Type -AssemblyName System.Drawing
  $pimg = [System.Drawing.Image]::FromFile($text)
  Write-Host "    落盘文件: $($f.FullName) $($f.Length) 字节 $($pimg.Width)x$($pimg.Height)"
  $pimg.Dispose()
} elseif ($text) {
  Write-Host "    落盘文件: 剪贴板里的路径不存在 -> $text"
}
Write-Host "[完成] 截图存于 $out"
