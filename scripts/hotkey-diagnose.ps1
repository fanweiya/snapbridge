$ErrorActionPreference = "Continue"
Add-Type -TypeDefinition @"
using System;
using System.Text;
using System.Runtime.InteropServices;
public class DG {
  public delegate bool EnumProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc cb, IntPtr l);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool GetWindowRect(IntPtr h, out RECT r);
  [DllImport("user32.dll")] public static extern int GetClassNameW(IntPtr h, StringBuilder s, int n);
  [DllImport("user32.dll")] public static extern int GetWindowTextW(IntPtr h, StringBuilder s, int n);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern void keybd_event(byte b, byte s, uint f, IntPtr e);
  public struct RECT { public int Left, Top, Right, Bottom; }

  public static string Fullscreen(uint pid) {
    var sb = new StringBuilder();
    EnumWindows(delegate(IntPtr h, IntPtr l) {
      uint p; GetWindowThreadProcessId(h, out p);
      if (p != pid || !IsWindowVisible(h)) return true;
      RECT r; GetWindowRect(h, out r);
      var c = new StringBuilder(256); GetClassNameW(h, c, 256);
      var t = new StringBuilder(256); GetWindowTextW(h, t, 256);
      sb.AppendLine(String.Format("  hwnd={0} {1} '{2}' {3}x{4} @({5},{6})", h, c, t, r.Right-r.Left, r.Bottom-r.Top, r.Left, r.Top));
      return true;
    }, IntPtr.Zero);
    return sb.ToString().TrimEnd();
  }

  public static void Combo(byte[] mods, byte vk) {
    foreach (var m in mods) keybd_event(m, 0, 0, IntPtr.Zero);
    keybd_event(vk, 0, 0, IntPtr.Zero);
    System.Threading.Thread.Sleep(80);
    keybd_event(vk, 0, 2, IntPtr.Zero);
    for (int i = mods.Length - 1; i >= 0; i--) keybd_event(mods[i], 0, 2, IntPtr.Zero);
  }
}
"@

Write-Host "=== ShareX / 旧进程"
Get-Process -Name "ShareX","mygo-shot" -ErrorAction SilentlyContinue | Select-Object Id, ProcessName, Path | Format-List

$p = Get-Process -Name "SnapBridge" -ErrorAction SilentlyContinue
if (-not $p) { Write-Host "SnapBridge 未运行"; exit 1 }
$pid2 = [uint32]$p.Id
Write-Host "=== SnapBridge PID $pid2，启动于 $($p.StartTime)"

$trials = @(
  @{ name = "PrintScreen"; mods = @(); vk = 0x2C },
  @{ name = "Ctrl+PrintScreen"; mods = @(0x11); vk = 0x2C },
  @{ name = "Alt+PrintScreen"; mods = @(0x12); vk = 0x2C },
  @{ name = "Ctrl+Shift+A"; mods = @(0x11, 0x10); vk = 0x41 },
  @{ name = "F1"; mods = @(); vk = 0x70 }
)

foreach ($t in $trials) {
  [DG]::Combo([byte[]]$t.mods, [byte]$t.vk)
  Start-Sleep -Milliseconds 1400
  $w = [DG]::Fullscreen($pid2)
  if ($w) {
    Write-Host "[$($t.name)] 唤起成功:"
    Write-Host $w
    [DG]::Combo(@(), 0x1B)  # Esc 关闭
    Start-Sleep -Milliseconds 900
  } else {
    Write-Host "[$($t.name)] 无窗口"
  }
}
