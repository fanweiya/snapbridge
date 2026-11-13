<p align="center">
  <img src="resources/icon.png" width="128" alt="SnapBridge">
</p>

<h1 align="center">SnapBridge</h1>

<p align="center">Windows 截图标注工具，一次复制同时带上图片与文件路径。</p>

---

## 它解决的问题

截图之后要粘贴到不同的地方，剪贴板里得装不同的东西：

- 粘贴到微信、QQ、Notion、Word、飞书，需要位图；
- 粘贴到终端、代码编辑器、工单系统、SSH 客户端，需要文件路径。

绝大多数截图工具只能二选一。想让终端拿到路径，你得先保存、再打开资源管理器、再复制、再切回终端；想让聊天窗口拿到图片，又得重新截一次。

SnapBridge 把这两份数据一次写进剪贴板：`PNG` 位图、`CF_HDROP` 文件、纯文本路径三份同时存在。粘贴到哪个程序，由接收方自己挑它能读的那一份。终端拿到 `C:\Users\you\Pictures\SnapBridge\2026-11\shot_20261113_141113.204.png`，聊天窗口拿到图片，全程一次操作。

## 快捷键

默认键位与 ShareX 完全重合，从 ShareX 迁过来不用改肌肉记忆。

| 快捷键 | 动作 |
| --- | --- |
| `PrintScreen` | 区域截图，拖拽框选后进入标注 |
| `Ctrl + PrintScreen` | 全屏截图 |
| `Alt + PrintScreen` | 活动窗口截图 |
| `Ctrl + Shift + A` | 区域截图（备用键） |
| `F1` | 区域截图（备用键） |

### 与 Windows 自带截图键的关系

Windows 11 有一个开关：「设置 → 辅助功能 → 键盘 → 使用 Print Screen 键打开屏幕截图」，默认开启。开启状态下，这个键归截图工具所有，普通热键注册拿不到，低级键盘钩子也拦不住，因为截图工具通过 Raw Input 取按键，不走钩子链。

ShareX 的处理方式写在它的安装脚本里（`ShareX.Setup/InnoSetup/ShareX-setup.iss`）：把注册表 `HKCU\Control Panel\Keyboard\PrintScreenKeyForSnippingEnabled` 写成 `0`，让这个键回到应用程序手里，然后靠自己的键盘钩子监听。

SnapBridge 做同样的事，只是没藏在安装器里：

- 启动时检测该值，发现系统占着就自动改成 `0`，并在钩子里接管 `PrintScreen`；
- 托盘菜单里的「接管 Print Screen 键」显示当前归属，随时可以切回去；
- 切回去之后系统截图工具恢复响应，SnapBridge 的 `PrintScreen` 随之失效。

`Win + Shift + S` 走的是另一条路径，不受这个开关影响，任何时候都能唤出系统截图。想两个都用，就保留系统接管、用 `Ctrl + Shift + A` 或 `F1` 唤起 SnapBridge。

## 标注与输出

框选之后工具栏贴在选区下方，八向手柄可以调整选区大小，选区内部保持原始画质。

工具：矩形、箭头、画笔、文字、序号图钉、荧光高亮、马赛克。

| 按键 | 作用 |
| --- | --- |
| `V` / `R` / `A` / `F` / `T` / `I` / `H` / `B` | 依次切换选择、矩形、箭头、画笔、文字、序号、高亮、马赛克 |
| Ctrl + Z / Ctrl + Y | 撤销 / 重做 |
| `Backspace` | 撤销上一笔 |
| `Ctrl + A` | 全选整个画面 |
| `Ctrl + S` | 保存文件 |
| `Enter` 或双击 | 复制到剪贴板并关闭 |
| `Ctrl + Shift + C` | 只复制文件路径 |
| `Esc` | 取消，不产生任何文件 |

颜色六种，笔画三档。鼠标移动时右上角有放大镜显示像素坐标，方便对齐细节。

## 剪贴板三种模式

托盘右键菜单可以切换，也可以在截图界面右下角点「模式」胶囊循环切换。

| 模式 | 剪贴板内容 | 适合 |
| --- | --- | --- |
| 图片 + 路径 | 位图、文件、文本路径 | 默认，任何程序都能粘 |
| 只要图片 | 位图 | 只想发图，不想在聊天里冒出文件名 |
| 只要路径 | 文件、文本路径 | 大量截图喂给脚本或命令行工具 |

## 后台驻留

启动后主窗口不出现，只在托盘放一个图标。托盘菜单：

- 三种截图入口，带加速键提示；
- 复制内容切换；
- 打开截图目录；
- 退出。

退出只能通过托盘菜单，关闭窗口不会结束进程。

## 截图存放

```
%USERPROFILE%\Pictures\SnapBridge\<年-月>\shot_<年月日>_<时分秒>.<毫秒>.png
```

按月分目录，文件名带毫秒，连续截图不会互相覆盖。

## 安装

从 Releases 下载 `snapbridge-setup-0.1.0-amd64.exe` 安装，或者直接用 `SnapBridge.exe` 单文件，它不需要安装、不需要运行时，拷到哪都能跑。首次运行 Windows 可能提示 SmartScreen，选「更多信息 → 仍要运行」。可执行文件没有代码签名。

程序依赖系统的 WebView2 运行时，Windows 11 自带，Windows 10 需要装一次 [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)。

### 开机自启

程序本身不写启动项。需要自启时把快捷方式放进：

```
%APPDATA%\Microsoft\Windows\Start Menu\Programs\Startup
```

## 从源码构建

需要 Go 1.27 以上、Bun 1.3 以上。装 MyGo 编译器：

```powershell
go install github.com/egoist/mygo/cmd/mygo@latest
```

`go install` 把 `mygo.exe` 放进 `%USERPROFILE%\go\bin`。`build.ps1` 会自己把 `%USERPROFILE%\go\bin`、`%USERPROFILE%\sdk\go\bin`、`%USERPROFILE%\.bun\bin`、`C:\Program Files\Go\bin` 补进 `PATH`，手工配过环境变量也能正常跑。

拉代码后构建：

```powershell
git clone https://github.com/fanweiya/snapbridge.git
cd snapbridge
bun install
powershell -ExecutionPolicy Bypass -File build.ps1
```

产物在 `build\windows-amd64\`：

- `SnapBridge.exe` 单文件可执行程序；
- `snapbridge-setup-0.1.0-amd64.exe` NSIS 安装包。

前端热更新：

```powershell
mygo dev
```

改了 `main.go` 里导出的方法之后要跑 `mygo generate` 重新生成 `src/mygo.ts` 类型绑定，`mygo build` 会自己做这一步。

### 重新生成图标

`resources/icon.png` 是 1024 像素的母版，`mygo build` 会按 16/24/32/48/64/128/256 生成 exe 内嵌图标。母版由脚本绘制：

```bash
python3 scripts/make_icons.py
```

脚本同时输出 `resources/icon.ico`（多尺寸 ICO，`.ico` 里的每个尺寸都是独立渲染的 PNG 载荷）、`tray-32.png`（托盘图标，透明底、加宽线条）和 `icon-preview.png`（各尺寸并排的核对图）。

## 冒烟测试

```powershell
powershell -ExecutionPolicy Bypass -File scripts\smoke-test.ps1
```

脚本会启动程序、模拟按下 `PrintScreen`、用鼠标拖出 500×400 的选区、按回车复制，然后回读剪贴板文本与位图，并检查保存到磁盘的 PNG 尺寸与字节数。结果截图放在 `build\smoke\`。

## 实现要点

| 部分 | 做法 |
| --- | --- |
| 抓屏 | Win32 GDI，`BitBlt` 从屏幕 DC 取像素，`GetDIBits` 转成位图，按虚拟桌面坐标拼合多显示器 |
| 精度 | 进程启用 `DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2`，高分屏按物理像素截图，不做系统拉伸 |
| 界面 | WebView2 承载 HTML + Canvas，无边框全屏置顶窗口 |
| 选区 | 双层 Canvas：底层切片原始截图保证清晰度，上层绘制矢量标注 |
| 热键 | `PrintScreen` 走 `WH_KEYBOARD_LL` 低级钩子，其余走 `RegisterHotKey` |
| 剪贴板 | 一次写入三种格式：`image/png`（平台层转成 `CF_DIB`）、`CF_HDROP` 文件列表、`CF_UNICODETEXT` |
| 打包 | MyGo 把前端资源嵌入二进制，附带 NSIS 安装包 |

技术组合：Go 1.27、WebView2、TypeScript、HTML5 Canvas、Vite。

## 目录结构

```
snapbridge/
├── main.go              Go 后端：托盘、热键、剪贴板、窗口生命周期
├── capture_windows.go   Win32 GDI 屏幕与窗口抓取
├── hotkey_windows.go    PrintScreen 低级键盘钩子
├── icon.go              托盘图标嵌入
├── index.html           遮罩层与工具栏结构
├── src/
│   ├── main.ts          选区、标注、放大镜、提交逻辑
│   ├── style.css        遮罩与工具栏样式
│   └── mygo.ts          mygo generate 生成的前后端绑定
├── resources/           图标母版、ICO、托盘图标
├── scripts/
│   ├── make_icons.py    图标生成脚本
│   ├── smoke-test.ps1   端到端冒烟测试
│   └── hotkey-diagnose.ps1  逐个热键的唤起诊断
├── mygo.config.ts       应用名、标识符、图标、构建配置
└── build.ps1            打包脚本
```

## 已知限制

- 只在 Windows 上构建，抓屏与热键都调用 Win32 API，没有跨平台实现。
- 多显示器按虚拟桌面拼成一张大图，超高分辨率时内存占用随像素总量增长。
- 马赛克与高亮在提交时才合成到导出图，撤销只作用于标注对象。
- 没有 OCR、没有贴图钉屏、没有自动上传。

## 许可

私有项目，保留所有权利。
